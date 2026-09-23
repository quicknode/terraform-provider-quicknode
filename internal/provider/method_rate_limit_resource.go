package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

const (
	limiterEnabled  = "enabled"
	limiterDisabled = "disabled"
)

var limiterIntervals = []string{"second", "minute", "hour"}

var _ resource.Resource = (*methodRateLimitResource)(nil)
var _ resource.ResourceWithConfigure = (*methodRateLimitResource)(nil)
var _ resource.ResourceWithImportState = (*methodRateLimitResource)(nil)

type methodRateLimitResource struct {
	client *client.Client
}

type methodRateLimitResourceModel struct {
	ID         types.String `tfsdk:"id"`
	EndpointID types.String `tfsdk:"endpoint_id"`
	Methods    types.Set    `tfsdk:"methods"`
	Rate       types.Int64  `tfsdk:"rate"`
	Interval   types.String `tfsdk:"interval"`
	Enabled    types.Bool   `tfsdk:"enabled"`
}

func NewMethodRateLimitResource() resource.Resource {
	return &methodRateLimitResource{}
}

func (r *methodRateLimitResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_endpoint_method_rate_limit"
}

func (r *methodRateLimitResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A rate limit on a named set of RPC methods, applied on top of the endpoint-wide limits in `quicknode_endpoint_rate_limits`.\n\n" +
			"Use it to keep a handful of expensive calls, such as `eth_getLogs` over wide block ranges, from consuming the endpoint's whole budget.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Rate limiter id assigned by Quicknode.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"endpoint_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Endpoint the limiter applies to.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"methods": schema.SetAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "RPC methods the limit counts, for example `eth_getLogs`.",
				Validators:          []validator.Set{setvalidator.SizeAtLeast(1)},
			},
			"rate": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Requests allowed across those methods per interval.",
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
			},
			"interval": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Window the rate applies to: `second`, `minute` or `hour`. Changing it replaces the limiter, because the Admin API's update route does not accept an interval.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators:          []validator.String{stringvalidator.OneOf(limiterIntervals...)},
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether the limiter is applied. Disabling keeps the definition in place, which suits turning a limit off during an incident without losing it.",
			},
		},
	}
}

func (r *methodRateLimitResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("The method rate limit resource expected providerData, got %T.", req.ProviderData))
		return
	}
	r.client = data.Client
}

func (r *methodRateLimitResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan methodRateLimitResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	methods, diags := methodNames(ctx, plan.Methods)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpointID := plan.EndpointID.ValueString()
	created, err := r.client.AddMethodRateLimit(ctx, endpointID, client.MethodRateLimit{
		Methods:  methods,
		Rate:     int(plan.Rate.ValueInt64()),
		Interval: plan.Interval.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Could not create the method rate limit", err.Error())
		return
	}

	plan.ID = types.StringValue(created.ID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The create route takes no status, so a limiter that is meant to start
	// disabled needs a second call.
	if !plan.Enabled.ValueBool() {
		if err := r.client.UpdateMethodRateLimit(ctx, endpointID, created.ID, client.MethodRateLimit{
			Methods: methods,
			Rate:    int(plan.Rate.ValueInt64()),
			Status:  limiterDisabled,
		}); err != nil {
			resp.Diagnostics.AddError("Created the method rate limit but could not disable it", err.Error())
		}
	}
}

func (r *methodRateLimitResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state methodRateLimitResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	limiters, err := r.client.ListMethodRateLimits(ctx, state.EndpointID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read the endpoint's method rate limits", err.Error())
		return
	}

	for _, limiter := range limiters {
		if limiter.ID != state.ID.ValueString() {
			continue
		}
		methods, diags := methodSet(methodsPreservingCase(ctx, limiter.Methods, state.Methods))
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		state.Methods = methods
		state.Rate = types.Int64Value(int64(limiter.Rate))
		state.Interval = types.StringValue(limiter.Interval)
		state.Enabled = types.BoolValue(limiter.Status != limiterDisabled)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}
	resp.State.RemoveResource(ctx)
}

func (r *methodRateLimitResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state methodRateLimitResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	methods, diags := methodNames(ctx, plan.Methods)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.ID = state.ID
	status := limiterEnabled
	if !plan.Enabled.ValueBool() {
		status = limiterDisabled
	}
	if err := r.client.UpdateMethodRateLimit(ctx, state.EndpointID.ValueString(), state.ID.ValueString(), client.MethodRateLimit{
		Methods: methods,
		Rate:    int(plan.Rate.ValueInt64()),
		Status:  status,
	}); err != nil {
		resp.Diagnostics.AddError("Could not update the method rate limit", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *methodRateLimitResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state methodRateLimitResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.RemoveMethodRateLimit(ctx, state.EndpointID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Could not remove the method rate limit", err.Error())
	}
}

// ImportState takes "<endpoint id>/<limiter id>". A limiter has no natural
// name, so it is addressed by id.
func (r *methodRateLimitResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	endpointID, limiterID, found := strings.Cut(req.ID, "/")
	if !found || endpointID == "" || limiterID == "" {
		resp.Diagnostics.AddError(
			"Unexpected import address",
			fmt.Sprintf("Import a method rate limit as \"<endpoint id>/<limiter id>\", for example \"123456/a1b2c3d4-...\". Got %q.", req.ID),
		)
		return
	}

	limiters, err := r.client.ListMethodRateLimits(ctx, endpointID)
	if err != nil {
		resp.Diagnostics.AddError("Could not read the endpoint's method rate limits", err.Error())
		return
	}

	for _, limiter := range limiters {
		if limiter.ID != limiterID {
			continue
		}
		methods, diags := methodSet(limiter.Methods)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		state := methodRateLimitResourceModel{
			ID:         types.StringValue(limiter.ID),
			EndpointID: types.StringValue(endpointID),
			Methods:    methods,
			Rate:       types.Int64Value(int64(limiter.Rate)),
			Interval:   types.StringValue(limiter.Interval),
			Enabled:    types.BoolValue(limiter.Status != limiterDisabled),
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}
	resp.Diagnostics.AddError("No matching method rate limit", fmt.Sprintf("Endpoint %s has no method rate limit with the id %q.", endpointID, limiterID))
}

// methodsPreservingCase keeps the casing the configuration wrote. This route
// lowercases the method names it stores, so reading them back verbatim would
// leave a diff that applying never settles. A method the prior state does not
// hold is kept as the API returned it, so a real change is still detected.
func methodsPreservingCase(ctx context.Context, fromAPI []string, prior types.Set) []string {
	if prior.IsNull() || prior.IsUnknown() {
		return fromAPI
	}
	priorNames, diags := methodNames(ctx, prior)
	if diags.HasError() {
		return fromAPI
	}

	configured := make(map[string]string, len(priorNames))
	for _, name := range priorNames {
		configured[strings.ToLower(name)] = name
	}

	preserved := make([]string, 0, len(fromAPI))
	for _, name := range fromAPI {
		if original, ok := configured[strings.ToLower(name)]; ok {
			preserved = append(preserved, original)
			continue
		}
		preserved = append(preserved, name)
	}
	return preserved
}
