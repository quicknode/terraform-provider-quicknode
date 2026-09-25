package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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

const (
	statusActive = "active"
	statusPaused = "paused"
)

var _ resource.Resource = (*endpointResource)(nil)
var _ resource.ResourceWithConfigure = (*endpointResource)(nil)
var _ resource.ResourceWithImportState = (*endpointResource)(nil)
var _ resource.ResourceWithModifyPlan = (*endpointResource)(nil)

type endpointResource struct {
	client *client.Client
	chains []client.Chain
}

type endpointResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Chain           types.String `tfsdk:"chain"`
	Network         types.String `tfsdk:"network"`
	Label           types.String `tfsdk:"label"`
	Status          types.String `tfsdk:"status"`
	Multichain      types.Bool   `tfsdk:"multichain"`
	Tags            types.Set    `tfsdk:"tags"`
	SafeHTTPURL     types.String `tfsdk:"safe_http_url"`
	SafeWSSURL      types.String `tfsdk:"safe_wss_url"`
	SecurityOptions types.Object `tfsdk:"security_options"`
	IPCustomHeader  types.String `tfsdk:"ip_custom_header"`
}

func NewEndpointResource() resource.Resource {
	return &endpointResource{}
}

func (r *endpointResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_endpoint"
}

func (r *endpointResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Quicknode RPC endpoint on a chain and network.\n\n" +
			"`safe_http_url` and `safe_wss_url` carry the literal `REPLACE_WITH_TOKEN` where the credential belongs, so they are safe to log or display " +
			"while keeping the real URL's shape, including any path suffix the chain appends.\n\n" +
			"The resource does not track the endpoint's tokens or the URLs that carry them, because adding or removing a token changes both. " +
			"Read a working URL from `data.quicknode_endpoint_urls`, or from the `quicknode_endpoint_token` that issued the credential.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Endpoint id.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"chain": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Chain slug, for example `eth`, `base`, `arb`, `sol`. Slugs are often abbreviations of the chain's name; read `data.quicknode_chains` for the full list.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"network": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Network slug, for example `mainnet`, `base-sepolia`, `arbitrum-mainnet`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"label": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Descriptive label for the endpoint. Labels are not unique and do not identify the endpoint. Removing the attribute clears the label.",
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"status": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "`active` or `paused`. Required, so importing an endpoint never changes whether it serves traffic.",
				Validators:          []validator.String{stringvalidator.OneOf(statusActive, statusPaused)},
			},
			"multichain": schema.BoolAttribute{
				Required:            true,
				MarkdownDescription: "Whether the endpoint serves more than one network. Required, so importing an endpoint never changes which networks it serves.",
			},
			"tags": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Tag labels applied to the endpoint. Omitting the attribute removes every tag the provider finds on the endpoint.",
			},
			"safe_http_url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The HTTPS URL with the auth token replaced by `REPLACE_WITH_TOKEN`. Safe to log or display.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"safe_wss_url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The WebSocket URL with the auth token replaced by `REPLACE_WITH_TOKEN`, or null on chains without WebSocket support.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"security_options": securityOptionsSchema(),
			"ip_custom_header": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Name of the header the endpoint reads the caller's IP address from, for example `X-Real-IP`. Set it when calls arrive through a proxy, so IP restrictions see the original caller's address and not the proxy's.",
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
		},
	}
}

func (r *endpointResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("The endpoint resource expected providerData, got %T.", req.ProviderData))
		return
	}
	r.client = data.Client
	r.chains = data.Chains
}

// ModifyPlan rejects an unknown chain or network at plan time, before an apply
// is already partway through creating something.
func (r *endpointResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var config endpointResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if config.Chain.IsUnknown() || config.Network.IsUnknown() {
		return
	}

	problem := validateChainNetwork(r.chains, config.Chain.ValueString(), config.Network.ValueString())
	if problem == nil {
		return
	}
	resp.Diagnostics.AddAttributeError(path.Root(problem.Attribute), problem.Summary, problem.Detail)
}

func (r *endpointResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan endpointResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateEndpoint(ctx, plan.Chain.ValueString(), plan.Network.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not create the Quicknode endpoint", err.Error())
		return
	}

	plan.ID = types.StringValue(created.ID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !plan.Label.IsNull() || created.Label != "" {
		if err := r.client.SetEndpointLabel(ctx, created.ID, plan.Label.ValueString()); err != nil {
			resp.Diagnostics.AddError(
				"Created the endpoint but could not set its label",
				fmt.Sprintf("Endpoint %s exists and is tracked in state. %s", created.ID, err.Error()),
			)
			return
		}
	}

	if plan.Multichain.ValueBool() {
		if err := r.client.SetEndpointMultichain(ctx, created.ID, true); err != nil {
			resp.Diagnostics.AddError("Created the endpoint but could not enable multichain", err.Error())
			return
		}
	}

	if plan.Status.ValueString() == statusPaused {
		if err := r.client.SetEndpointStatus(ctx, created.ID, statusPaused); err != nil {
			resp.Diagnostics.AddError("Created the endpoint but could not pause it", err.Error())
			return
		}
	}

	resp.Diagnostics.Append(r.applySecurity(ctx, created.ID, plan.SecurityOptions, plan.IPCustomHeader, types.StringNull())...)
	if resp.Diagnostics.HasError() {
		return
	}

	wanted, diags := tagLabels(ctx, plan.Tags)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	for _, label := range wanted {
		if err := r.client.AddEndpointTag(ctx, created.ID, label); err != nil {
			resp.Diagnostics.AddError("Created the endpoint but could not tag it", err.Error())
			return
		}
	}

	r.readInto(ctx, created.ID, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *endpointResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state endpointResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint, err := r.client.GetEndpoint(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read the Quicknode endpoint", err.Error())
		return
	}

	resp.Diagnostics.Append(applyEndpoint(endpoint, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *endpointResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state endpointResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	plan.ID = state.ID

	if !plan.Label.Equal(state.Label) {
		if err := r.client.SetEndpointLabel(ctx, id, plan.Label.ValueString()); err != nil {
			resp.Diagnostics.AddError("Could not update the endpoint label", err.Error())
			return
		}
	}

	if !plan.Multichain.Equal(state.Multichain) {
		if err := r.client.SetEndpointMultichain(ctx, id, plan.Multichain.ValueBool()); err != nil {
			resp.Diagnostics.AddError("Could not update endpoint multichain", err.Error())
			return
		}
	}

	if !plan.Status.Equal(state.Status) {
		if err := r.client.SetEndpointStatus(ctx, id, plan.Status.ValueString()); err != nil {
			resp.Diagnostics.AddError("Could not update the endpoint status", err.Error())
			return
		}
	}

	if !plan.SecurityOptions.Equal(state.SecurityOptions) || !plan.IPCustomHeader.Equal(state.IPCustomHeader) {
		resp.Diagnostics.Append(r.applySecurity(ctx, id, plan.SecurityOptions, plan.IPCustomHeader, state.IPCustomHeader)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	if !plan.Tags.Equal(state.Tags) {
		resp.Diagnostics.Append(r.reconcileTags(ctx, id, plan.Tags)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	r.readInto(ctx, id, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *endpointResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state endpointResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteEndpoint(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Could not delete the Quicknode endpoint", err.Error())
	}
}

func (r *endpointResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// readInto refreshes only the attributes the API computes. Create and Update
// leave configured attributes at their planned values, because Terraform
// rejects an applied state that disagrees with the plan.
func (r *endpointResource) readInto(ctx context.Context, id string, model *endpointResourceModel, diags *diag.Diagnostics) {
	endpoint, err := r.client.GetEndpoint(ctx, id)
	if err != nil {
		diags.AddError("Could not read the Quicknode endpoint back", err.Error())
		return
	}
	model.ID = types.StringValue(endpoint.ID)
	applyEndpointURLs(endpoint, model)

	options, optionDiags := securityOptionsObject(endpoint.Security)
	diags.Append(optionDiags...)
	model.SecurityOptions = options
}

// applySecurity writes the settable toggles and the custom IP header. The
// previous header is needed because clearing the attribute becomes a delete
// call, not an empty write.
func (r *endpointResource) applySecurity(ctx context.Context, id string, options types.Object, header, previousHeader types.String) diag.Diagnostics {
	patch, diags := securityOptionsPatch(ctx, options)
	if diags.HasError() {
		return diags
	}
	if err := r.client.SetSecurityOptions(ctx, id, patch); err != nil {
		diags.AddError("Could not update the endpoint security options", err.Error())
		return diags
	}

	switch {
	case !header.IsNull() && !header.IsUnknown():
		if err := r.client.SetIPCustomHeader(ctx, id, header.ValueString()); err != nil {
			diags.AddError("Could not set the endpoint's custom IP header", err.Error())
		}
	case !previousHeader.IsNull():
		if err := r.client.DeleteIPCustomHeader(ctx, id); err != nil {
			diags.AddError("Could not clear the endpoint's custom IP header", err.Error())
		}
	}
	return diags
}

func applyEndpoint(endpoint *client.Endpoint, state *endpointResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	state.ID = types.StringValue(endpoint.ID)
	state.Chain = types.StringValue(endpoint.Chain)
	state.Network = types.StringValue(endpoint.Network)
	state.Status = types.StringValue(endpoint.Status)
	state.Multichain = types.BoolValue(endpoint.Multichain)
	applyEndpointURLs(endpoint, state)

	options, optionDiags := securityOptionsObject(endpoint.Security)
	diags.Append(optionDiags...)
	state.SecurityOptions = options
	state.IPCustomHeader = stringOrNull(endpoint.Security.IPCustomHeader)
	state.Label = stringOrNull(endpoint.Label)

	if len(endpoint.Tags) == 0 && state.Tags.IsNull() {
		return diags
	}
	labels := make([]attr.Value, 0, len(endpoint.Tags))
	for _, tag := range endpoint.Tags {
		labels = append(labels, types.StringValue(tag.Label))
	}
	tagSet, tagDiags := types.SetValue(types.StringType, labels)
	diags.Append(tagDiags...)
	state.Tags = tagSet
	return diags
}

// applyEndpointURLs maps empty URLs to null, so a chain without WebSocket
// support reports safe_wss_url as absent.
func applyEndpointURLs(endpoint *client.Endpoint, model *endpointResourceModel) {
	model.SafeHTTPURL = stringOrNull(endpoint.SafeHTTPURL)
	model.SafeWSSURL = stringOrNull(endpoint.SafeWSSURL)
}

func stringOrNull(value string) types.String {
	if value == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}

func (r *endpointResource) reconcileTags(ctx context.Context, id string, planned types.Set) diag.Diagnostics {
	var diags diag.Diagnostics

	wanted, tagDiags := tagLabels(ctx, planned)
	diags.Append(tagDiags...)
	if diags.HasError() {
		return diags
	}

	endpoint, err := r.client.GetEndpoint(ctx, id)
	if err != nil {
		diags.AddError("Could not read the endpoint's current tags", err.Error())
		return diags
	}

	current := make(map[string]int64, len(endpoint.Tags))
	for _, tag := range endpoint.Tags {
		current[tag.Label] = tag.ID
	}
	keep := make(map[string]struct{}, len(wanted))
	for _, label := range wanted {
		keep[label] = struct{}{}
	}

	for label, tagID := range current {
		if _, ok := keep[label]; ok {
			continue
		}
		if err := r.client.RemoveEndpointTag(ctx, id, tagID); err != nil {
			diags.AddError("Could not remove an endpoint tag", fmt.Sprintf("tag %q: %s", label, err.Error()))
			return diags
		}
	}
	for _, label := range wanted {
		if _, ok := current[label]; ok {
			continue
		}
		if err := r.client.AddEndpointTag(ctx, id, label); err != nil {
			diags.AddError("Could not add an endpoint tag", fmt.Sprintf("tag %q: %s", label, err.Error()))
			return diags
		}
	}
	return diags
}

func tagLabels(ctx context.Context, tags types.Set) ([]string, diag.Diagnostics) {
	if tags.IsNull() || tags.IsUnknown() {
		return nil, nil
	}
	var labels []string
	diags := tags.ElementsAs(ctx, &labels, false)
	return labels, diags
}
