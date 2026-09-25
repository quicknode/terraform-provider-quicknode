package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

// securityEntryKind describes one of the allowlists whose entries are created
// and deleted but never edited. The three share everything except the name of
// the attribute holding the value and the routes behind it.
type securityEntryKind struct {
	typeName    string
	attribute   string
	toggle      string
	noun        string
	subject     string
	description string
	valueDoc    string

	add    func(*client.Client, context.Context, string, string) (*client.SecurityEntry, error)
	remove func(*client.Client, context.Context, string, string) error
	list   func(*client.EndpointSecurity) []client.SecurityEntry
}

var securityEntryKinds = []securityEntryKind{
	{
		typeName:    "endpoint_ip",
		attribute:   "ip",
		toggle:      "ips",
		noun:        "IP address",
		subject:     "IP address filtering",
		description: "An IP address allowed to call a Quicknode endpoint.",
		valueDoc:    "IP address or CIDR range allowed to call the endpoint.",
		add: func(c *client.Client, ctx context.Context, endpointID, value string) (*client.SecurityEntry, error) {
			return c.AddEndpointIP(ctx, endpointID, value)
		},
		remove: func(c *client.Client, ctx context.Context, endpointID, entryID string) error {
			return c.RemoveEndpointIP(ctx, endpointID, entryID)
		},
		list: func(security *client.EndpointSecurity) []client.SecurityEntry { return security.IPs },
	},
	{
		typeName:    "endpoint_domain_mask",
		attribute:   "domain",
		toggle:      "domain_masks",
		noun:        "domain mask",
		subject:     "Domain masking",
		description: "A custom domain that serves a Quicknode endpoint, so callers reach it without the Quicknode hostname.",
		valueDoc:    "Domain that serves the endpoint, for example `rpc.example.com`.",
		add: func(c *client.Client, ctx context.Context, endpointID, value string) (*client.SecurityEntry, error) {
			return c.AddDomainMask(ctx, endpointID, value)
		},
		remove: func(c *client.Client, ctx context.Context, endpointID, entryID string) error {
			return c.RemoveDomainMask(ctx, endpointID, entryID)
		},
		list: func(security *client.EndpointSecurity) []client.SecurityEntry { return security.DomainMasks },
	},
	{
		typeName:    "endpoint_referrer",
		attribute:   "referrer",
		toggle:      "referrers",
		noun:        "referrer",
		subject:     "Referrer filtering",
		description: "A referrer allowed to call a Quicknode endpoint. Referrer checks suit browser traffic, where the browser sets the header and the caller cannot.",
		valueDoc:    "Referrer URL allowed to call the endpoint, for example `https://app.example.com`.",
		add: func(c *client.Client, ctx context.Context, endpointID, value string) (*client.SecurityEntry, error) {
			return c.AddReferrer(ctx, endpointID, value)
		},
		remove: func(c *client.Client, ctx context.Context, endpointID, entryID string) error {
			return c.RemoveReferrer(ctx, endpointID, entryID)
		},
		list: func(security *client.EndpointSecurity) []client.SecurityEntry { return security.Referrers },
	},
}

func securityEntryResources() []func() resource.Resource {
	constructors := make([]func() resource.Resource, 0, len(securityEntryKinds))
	for _, kind := range securityEntryKinds {
		constructors = append(constructors, func() resource.Resource {
			return &securityEntryResource{kind: kind}
		})
	}
	return constructors
}

var _ resource.Resource = (*securityEntryResource)(nil)
var _ resource.ResourceWithConfigure = (*securityEntryResource)(nil)
var _ resource.ResourceWithImportState = (*securityEntryResource)(nil)

type securityEntryResource struct {
	kind   securityEntryKind
	client *client.Client
}

func (r *securityEntryResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.kind.typeName
}

func (r *securityEntryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: r.kind.description + "\n\n" +
			"The entry only takes effect once `security_options." + r.kind.toggle + "` is enabled on the endpoint. " +
			"Entries can be added before the toggle is turned on, which is the safe order for an endpoint already serving traffic.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Entry id assigned by Quicknode.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"endpoint_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Endpoint the entry belongs to.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			r.kind.attribute: schema.StringAttribute{
				Required:            true,
				MarkdownDescription: r.kind.valueDoc + " Changing it replaces the entry, because the Admin API has no route to edit one in place.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
		},
	}
}

func (r *securityEntryResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("The %s resource expected providerData, got %T.", r.kind.typeName, req.ProviderData))
		return
	}
	r.client = data.Client
}

func (r *securityEntryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var endpointID, value types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("endpoint_id"), &endpointID)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root(r.kind.attribute), &value)...)
	if resp.Diagnostics.HasError() {
		return
	}

	entry, err := r.kind.add(r.client, ctx, endpointID.ValueString(), value.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not add the "+r.kind.noun, err.Error()+r.hiddenEntryHint(ctx, err, endpointID.ValueString(), value.ValueString()))
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(entry.ID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("endpoint_id"), endpointID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(r.kind.attribute), value)...)
	resp.Diagnostics.Append(warnToggleDisabled(ctx, r.client, endpointID.ValueString(), r.kind.toggle, r.kind.subject)...)
}

func (r *securityEntryResource) hiddenEntryHint(ctx context.Context, err error, endpointID, value string) string {
	if !client.IsAlreadyExists(err) {
		return ""
	}
	security, readErr := r.client.GetEndpointSecurity(ctx, endpointID)
	if readErr != nil || securityToggleEnabled(security.Options, r.kind.toggle) {
		return ""
	}
	return fmt.Sprintf("\n\nThe %s already exists on endpoint %s, but security_options.%s is false, and the Admin API hides its entries while it is off. "+
		"Enable it, then import the entry with \"terraform import <address> %s/%s\".", r.kind.noun, endpointID, r.kind.toggle, endpointID, value)
}

func (r *securityEntryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var entryID, endpointID types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &entryID)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("endpoint_id"), &endpointID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	security, err := r.client.GetEndpointSecurity(ctx, endpointID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read the endpoint's "+r.kind.noun+" entries", err.Error())
		return
	}

	for _, entry := range r.kind.list(security) {
		if entry.ID != entryID.ValueString() {
			continue
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(r.kind.attribute), types.StringValue(entry.Value))...)
		return
	}
	// The security route omits a list entirely while its toggle is disabled,
	// so an entry that cannot be seen has not necessarily been deleted.
	// Dropping it from state here would have the next apply create a duplicate.
	if !securityToggleEnabled(security.Options, r.kind.toggle) {
		return
	}
	resp.State.RemoveResource(ctx)
}

// Update never runs: every attribute replaces the resource.
func (r *securityEntryResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}

func (r *securityEntryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var entryID, endpointID types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &entryID)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("endpoint_id"), &endpointID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.kind.remove(r.client, ctx, endpointID.ValueString(), entryID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Could not remove the "+r.kind.noun, err.Error())
	}
}

// ImportState takes "<endpoint id>/<value>". The address is the value itself,
// so nothing has to be looked up first.
func (r *securityEntryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	endpointID, value, found := strings.Cut(req.ID, "/")
	if !found || endpointID == "" || value == "" {
		resp.Diagnostics.AddError(
			"Unexpected import address",
			fmt.Sprintf("Import a %s as \"<endpoint id>/<%s>\", for example \"123456/%s\". Got %q.", r.kind.noun, r.kind.attribute, importExample(r.kind.attribute), req.ID),
		)
		return
	}

	security, err := r.client.GetEndpointSecurity(ctx, endpointID)
	if err != nil {
		resp.Diagnostics.AddError("Could not read the endpoint's "+r.kind.noun+" entries", err.Error())
		return
	}

	matches := make([]client.SecurityEntry, 0, 1)
	for _, entry := range r.kind.list(security) {
		if entry.Value == value {
			matches = append(matches, entry)
		}
	}
	switch len(matches) {
	case 0:
		if !securityToggleEnabled(security.Options, r.kind.toggle) {
			resp.Diagnostics.AddError(
				r.kind.subject+" is disabled on the endpoint",
				fmt.Sprintf("Endpoint %s has security_options.%s set to false, and the Admin API hides its %s entries while it is off. Enable it and import again.", endpointID, r.kind.toggle, r.kind.noun),
			)
			return
		}
		resp.Diagnostics.AddError(
			"No matching "+r.kind.noun,
			fmt.Sprintf("Endpoint %s has no %s entry with the value %q.", endpointID, r.kind.noun, value),
		)
		return
	case 1:
	default:
		resp.Diagnostics.AddError(
			"More than one matching "+r.kind.noun,
			fmt.Sprintf("Endpoint %s has %d %s entries with the value %q, so this address is ambiguous. Remove the duplicates, or import by writing the entry's id into state directly.", endpointID, len(matches), r.kind.noun, value),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(matches[0].ID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("endpoint_id"), types.StringValue(endpointID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(r.kind.attribute), types.StringValue(matches[0].Value))...)
}

func importExample(attribute string) string {
	switch attribute {
	case "ip":
		return "203.0.113.7"
	case "domain":
		return "rpc.example.com"
	}
	return "https://app.example.com"
}
