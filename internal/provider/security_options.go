package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

var securityOptionsAttrTypes = map[string]attr.Type{
	"tokens":           types.BoolType,
	"referrers":        types.BoolType,
	"jwts":             types.BoolType,
	"ips":              types.BoolType,
	"domain_masks":     types.BoolType,
	"hsts":             types.BoolType,
	"cors":             types.BoolType,
	"request_filters":  types.BoolType,
	"response_logging": types.BoolType,
}

type securityOptionsModel struct {
	Tokens      types.Bool `tfsdk:"tokens"`
	Referrers   types.Bool `tfsdk:"referrers"`
	JWTs        types.Bool `tfsdk:"jwts"`
	IPs         types.Bool `tfsdk:"ips"`
	DomainMasks types.Bool `tfsdk:"domain_masks"`
	HSTS        types.Bool `tfsdk:"hsts"`
	Cors        types.Bool `tfsdk:"cors"`

	RequestFilters  types.Bool `tfsdk:"request_filters"`
	ResponseLogging types.Bool `tfsdk:"response_logging"`
}

func securityOptionsSchema() schema.SingleNestedAttribute {
	settable := func(description string) schema.BoolAttribute {
		return schema.BoolAttribute{
			Optional:            true,
			Computed:            true,
			MarkdownDescription: description,
		}
	}
	return schema.SingleNestedAttribute{
		Optional: true,
		Computed: true,
		MarkdownDescription: "Which security mechanisms the endpoint enforces. Each toggle only decides whether a mechanism is applied; " +
			"the entries it applies to are separate resources, such as `quicknode_endpoint_ip`. " +
			"A toggle left out of the configuration keeps whatever value the endpoint already has.",
		Attributes: map[string]schema.Attribute{
			"tokens":       settable("Require one of the endpoint's auth tokens. New endpoints have this enabled."),
			"referrers":    settable("Restrict calls to the approved referrers. Add them with `quicknode_endpoint_referrer`."),
			"jwts":         settable("Require a signed JWT. Register signing keys with `quicknode_endpoint_jwt`."),
			"ips":          settable("Restrict calls to the approved IP addresses. Add them with `quicknode_endpoint_ip`."),
			"domain_masks": settable("Serve the endpoint from an approved custom domain. Add them with `quicknode_endpoint_domain_mask`."),
			"hsts":         settable("Send the HTTP Strict Transport Security header."),
			"cors":         settable("Apply Cross-Origin Resource Sharing policy. New endpoints have this enabled."),
			"request_filters": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether RPC method filtering is applied. Read-only: the Admin API turns this on when a `quicknode_endpoint_request_filter` exists and off when the last one is removed.",
			},
			"response_logging": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether responses are logged for the endpoint. Read-only: the account's plan sets it and it cannot be changed per endpoint.",
			},
		},
	}
}

// securityOptionsObject renders what the API reports. Every toggle is known
// after a read, including the two the provider cannot write.
func securityOptionsObject(options client.SecurityOptions) (types.Object, diag.Diagnostics) {
	return types.ObjectValue(securityOptionsAttrTypes, map[string]attr.Value{
		"tokens":           types.BoolValue(options.Tokens),
		"referrers":        types.BoolValue(options.Referrers),
		"jwts":             types.BoolValue(options.JWTs),
		"ips":              types.BoolValue(options.IPs),
		"domain_masks":     types.BoolValue(options.DomainMasks),
		"hsts":             types.BoolValue(options.HSTS),
		"cors":             types.BoolValue(options.Cors),
		"request_filters":  types.BoolValue(options.RequestFilters),
		"response_logging": types.BoolValue(options.ResponseLogging),
	})
}

// securityOptionsPatch collects the toggles worth writing. An unknown value is
// one Terraform will fill from the API, and a null value is one the
// configuration does not manage; neither belongs in the request body, because
// sending it would overwrite a setting nobody asked to change.
func securityOptionsPatch(ctx context.Context, planned types.Object) (client.SecurityOptionsPatch, diag.Diagnostics) {
	var patch client.SecurityOptionsPatch
	if planned.IsNull() || planned.IsUnknown() {
		return patch, nil
	}

	var model securityOptionsModel
	diags := planned.As(ctx, &model, basetypes.ObjectAsOptions{})
	if diags.HasError() {
		return patch, diags
	}

	for _, toggle := range []struct {
		value  types.Bool
		target **bool
	}{
		{model.Tokens, &patch.Tokens},
		{model.Referrers, &patch.Referrers},
		{model.JWTs, &patch.JWTs},
		{model.IPs, &patch.IPs},
		{model.DomainMasks, &patch.DomainMasks},
		{model.HSTS, &patch.HSTS},
		{model.Cors, &patch.Cors},
	} {
		if toggle.value.IsNull() || toggle.value.IsUnknown() {
			continue
		}
		wanted := toggle.value.ValueBool()
		*toggle.target = &wanted
	}
	return patch, diags
}

// securityToggleEnabled reports one toggle by its resource-facing name, which
// the entry resources use to warn when they add something the endpoint is not
// enforcing.
func securityToggleEnabled(options client.SecurityOptions, name string) bool {
	switch name {
	case "tokens":
		return options.Tokens
	case "referrers":
		return options.Referrers
	case "jwts":
		return options.JWTs
	case "ips":
		return options.IPs
	case "domain_masks":
		return options.DomainMasks
	case "request_filters":
		return options.RequestFilters
	}
	return true
}

// warnToggleDisabled reports an entry the endpoint is not enforcing. The API
// accepts it either way, and building an allowlist before enabling the toggle
// is the safe order for an endpoint already serving traffic, so this warns
// and does not fail.
func warnToggleDisabled(ctx context.Context, quicknode *client.Client, endpointID, toggle, subject string) diag.Diagnostics {
	var diags diag.Diagnostics

	security, err := quicknode.GetEndpointSecurity(ctx, endpointID)
	if err != nil || securityToggleEnabled(security.Options, toggle) {
		return diags
	}
	diags.AddWarning(
		subject+" is disabled on the endpoint",
		fmt.Sprintf("Endpoint %s has security_options.%s set to false, so this entry is stored but not enforced. Set %s = true on the endpoint to apply it.", endpointID, toggle, toggle),
	)
	return diags
}
