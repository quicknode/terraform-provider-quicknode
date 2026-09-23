package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

var planDefaultAttrTypes = map[string]attr.Type{
	client.BucketRPS: types.Int64Type,
	client.BucketRPM: types.Int64Type,
	client.BucketRPD: types.Int64Type,
}

var _ resource.Resource = (*rateLimitsResource)(nil)
var _ resource.ResourceWithConfigure = (*rateLimitsResource)(nil)
var _ resource.ResourceWithImportState = (*rateLimitsResource)(nil)

type rateLimitsResource struct {
	client *client.Client
}

type rateLimitsResourceModel struct {
	ID          types.String `tfsdk:"id"`
	EndpointID  types.String `tfsdk:"endpoint_id"`
	RPS         types.Int64  `tfsdk:"rps"`
	RPM         types.Int64  `tfsdk:"rpm"`
	RPD         types.Int64  `tfsdk:"rpd"`
	PlanDefault types.Object `tfsdk:"plan_default"`
}

func NewRateLimitsResource() resource.Resource {
	return &rateLimitsResource{}
}

func (r *rateLimitsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_endpoint_rate_limits"
}

func (r *rateLimitsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	bucket := func(description string) schema.Int64Attribute {
		return schema.Int64Attribute{
			Optional:            true,
			MarkdownDescription: description,
			Validators:          []validator.Int64{int64validator.AtLeast(1)},
		}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Endpoint-wide request rate limits, one resource per endpoint.\n\n" +
			"Each bucket the Quicknode plan sets is reported under `plan_default`. A bucket set here overrides the plan default; " +
			"a bucket left out keeps the plan default, and removing one that was set returns that bucket to the plan default rather than leaving the override in place.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Same as `endpoint_id`. Rate limits are a property of the endpoint rather than a separate object.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"endpoint_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Endpoint the limits apply to.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"rps": bucket("Maximum requests per second. Omit to keep the plan default."),
			"rpm": bucket("Maximum requests per minute. Omit to keep the plan default."),
			"rpd": bucket("Maximum requests per day. Omit to keep the plan default."),
			"plan_default": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "What the account's Quicknode plan allows, before any override set here. A bucket the plan does not limit is reported as `-1`.",
				Attributes: map[string]schema.Attribute{
					client.BucketRPS: schema.Int64Attribute{Computed: true, MarkdownDescription: "Plan limit on requests per second."},
					client.BucketRPM: schema.Int64Attribute{Computed: true, MarkdownDescription: "Plan limit on requests per minute."},
					client.BucketRPD: schema.Int64Attribute{Computed: true, MarkdownDescription: "Plan limit on requests per day."},
				},
			},
		},
	}
}

func (r *rateLimitsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("The rate limits resource expected providerData, got %T.", req.ProviderData))
		return
	}
	r.client = data.Client
}

func (r *rateLimitsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan rateLimitsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpointID := plan.EndpointID.ValueString()
	if err := r.client.SetRateLimits(ctx, endpointID, overridesFrom(plan)); err != nil {
		resp.Diagnostics.AddError("Could not set the endpoint rate limits", err.Error())
		return
	}

	plan.ID = plan.EndpointID
	resp.Diagnostics.Append(r.readPlanDefaults(ctx, endpointID, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *rateLimitsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state rateLimitsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	limits, err := r.client.GetRateLimits(ctx, state.EndpointID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read the endpoint rate limits", err.Error())
		return
	}

	overrides, defaults := splitRateLimits(limits)
	state.ID = state.EndpointID
	state.RPS = bucketOrNull(overrides, client.BucketRPS)
	state.RPM = bucketOrNull(overrides, client.BucketRPM)
	state.RPD = bucketOrNull(overrides, client.BucketRPD)

	planDefault, diags := planDefaultObject(defaults)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.PlanDefault = planDefault
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *rateLimitsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state rateLimitsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpointID := state.EndpointID.ValueString()
	plan.ID = state.ID

	// A bucket dropped from the configuration has to lose its override, which
	// is a delete rather than a write: the patch route has no way to say
	// "return this bucket to the plan default".
	dropped := map[string]bool{
		client.BucketRPS: plan.RPS.IsNull() && !state.RPS.IsNull(),
		client.BucketRPM: plan.RPM.IsNull() && !state.RPM.IsNull(),
		client.BucketRPD: plan.RPD.IsNull() && !state.RPD.IsNull(),
	}
	if dropped[client.BucketRPS] || dropped[client.BucketRPM] || dropped[client.BucketRPD] {
		resp.Diagnostics.Append(r.dropOverrides(ctx, endpointID, dropped)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	if overrides := overridesFrom(plan); overrides.RPS != nil || overrides.RPM != nil || overrides.RPD != nil {
		if err := r.client.SetRateLimits(ctx, endpointID, overrides); err != nil {
			resp.Diagnostics.AddError("Could not update the endpoint rate limits", err.Error())
			return
		}
	}

	resp.Diagnostics.Append(r.readPlanDefaults(ctx, endpointID, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete returns every bucket to the plan default. Nothing is torn down,
// because the limits belong to the endpoint rather than to a separate object.
func (r *rateLimitsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state rateLimitsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	all := map[string]bool{client.BucketRPS: true, client.BucketRPM: true, client.BucketRPD: true}
	resp.Diagnostics.Append(r.dropOverrides(ctx, state.EndpointID.ValueString(), all)...)
}

func (r *rateLimitsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(req.ID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("endpoint_id"), types.StringValue(req.ID))...)
}

func (r *rateLimitsResource) dropOverrides(ctx context.Context, endpointID string, buckets map[string]bool) diag.Diagnostics {
	var diags diag.Diagnostics

	limits, err := r.client.GetRateLimits(ctx, endpointID)
	if client.IsNotFound(err) {
		return diags
	}
	if err != nil {
		diags.AddError("Could not read the endpoint rate limits", err.Error())
		return diags
	}

	for _, limit := range limits {
		if limit.Source != client.SourceUserOverride || limit.ID == "" || !buckets[limit.Bucket] {
			continue
		}
		if err := r.client.DeleteRateLimitOverride(ctx, endpointID, limit.ID); err != nil {
			diags.AddError("Could not return a rate limit bucket to the plan default", fmt.Sprintf("bucket %q: %s", limit.Bucket, err.Error()))
			return diags
		}
	}
	return diags
}

func (r *rateLimitsResource) readPlanDefaults(ctx context.Context, endpointID string, model *rateLimitsResourceModel) diag.Diagnostics {
	limits, err := r.client.GetRateLimits(ctx, endpointID)
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError("Could not read the endpoint rate limits back", err.Error())
		return diags
	}
	_, defaults := splitRateLimits(limits)

	planDefault, diags := planDefaultObject(defaults)
	model.PlanDefault = planDefault
	return diags
}

func overridesFrom(model rateLimitsResourceModel) client.RateLimitOverrides {
	var overrides client.RateLimitOverrides
	for _, bucket := range []struct {
		value  types.Int64
		target **int
	}{
		{model.RPS, &overrides.RPS},
		{model.RPM, &overrides.RPM},
		{model.RPD, &overrides.RPD},
	} {
		if bucket.value.IsNull() || bucket.value.IsUnknown() {
			continue
		}
		wanted := int(bucket.value.ValueInt64())
		*bucket.target = &wanted
	}
	return overrides
}

func splitRateLimits(limits []client.RateLimit) (overrides, defaults map[string]int) {
	overrides = make(map[string]int, 3)
	defaults = make(map[string]int, 3)
	for _, limit := range limits {
		switch limit.Source {
		case client.SourceUserOverride:
			overrides[limit.Bucket] = limit.Value
		case client.SourcePlanDefault:
			defaults[limit.Bucket] = limit.Value
		}
	}
	return overrides, defaults
}

func bucketOrNull(buckets map[string]int, name string) types.Int64 {
	value, ok := buckets[name]
	if !ok || value == client.RateLimitUnset {
		return types.Int64Null()
	}
	return types.Int64Value(int64(value))
}

func planDefaultObject(defaults map[string]int) (types.Object, diag.Diagnostics) {
	value := func(name string) attr.Value {
		limit, ok := defaults[name]
		if !ok {
			limit = client.RateLimitUnset
		}
		return types.Int64Value(int64(limit))
	}
	return types.ObjectValue(planDefaultAttrTypes, map[string]attr.Value{
		client.BucketRPS: value(client.BucketRPS),
		client.BucketRPM: value(client.BucketRPM),
		client.BucketRPD: value(client.BucketRPD),
	})
}
