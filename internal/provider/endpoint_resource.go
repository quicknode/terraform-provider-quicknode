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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

const (
	statusActive = "active"
	statusPaused = "paused"
)

var endpointTokenAttrTypes = map[string]attr.Type{
	"id":    types.StringType,
	"token": types.StringType,
}

var _ resource.Resource = (*endpointResource)(nil)
var _ resource.ResourceWithConfigure = (*endpointResource)(nil)
var _ resource.ResourceWithImportState = (*endpointResource)(nil)
var _ resource.ResourceWithModifyPlan = (*endpointResource)(nil)

type endpointResource struct {
	client *client.Client
	chains []client.Chain
}

type endpointResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Chain            types.String `tfsdk:"chain"`
	Network          types.String `tfsdk:"network"`
	Label            types.String `tfsdk:"label"`
	Status           types.String `tfsdk:"status"`
	Multichain       types.Bool   `tfsdk:"multichain"`
	Tags             types.Set    `tfsdk:"tags"`
	HTTPURL          types.String `tfsdk:"http_url"`
	WSSURL           types.String `tfsdk:"wss_url"`
	HTTPURLWithToken types.String `tfsdk:"http_url_with_token"`
	WSSURLWithToken  types.String `tfsdk:"wss_url_with_token"`
	Tokens           types.List   `tfsdk:"tokens"`
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
			"Pass `http_url_with_token` to anything that needs to make RPC calls. " +
			"`http_url` and `wss_url` have the credential removed and are safe to log or expose, but they are not usable endpoints: " +
			"the token does not sit at the end of the path on every chain, so rebuilding a URL by joining them to a token produces a broken address on chains that append a suffix.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Endpoint id.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"chain": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Chain slug, for example `eth`, `base`, `arb`, `sol`. Slugs are often abbreviations rather than the chain's name; read `data.quicknode_chains` for the full list.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"network": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Network slug, for example `mainnet`, `base-sepolia`, `arbitrum-mainnet`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"label": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Descriptive label for the endpoint. Labels are not unique and are not used to identify the endpoint.",
			},
			"status": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(statusActive),
				MarkdownDescription: "`active` or `paused`.",
				Validators:          []validator.String{stringvalidator.OneOf(statusActive, statusPaused)},
			},
			"multichain": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether the endpoint serves more than one network.",
			},
			"tags": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Tag labels applied to the endpoint. Omitting the attribute removes every tag the provider finds on the endpoint.",
			},
			"http_url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "HTTPS URL with the auth token removed. Safe to expose, but not a working endpoint.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"wss_url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "WebSocket URL with the auth token removed, or null on chains without WebSocket support.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"http_url_with_token": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "The working HTTPS endpoint, exactly as the Admin API returns it. Pass this to whatever makes RPC calls.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"wss_url_with_token": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "The working WebSocket endpoint, or null on chains without WebSocket support.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"tokens": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Auth tokens for the endpoint. An endpoint can carry several. Token values are stored in Terraform state, so keep state encrypted and remote.",
				PlanModifiers:       []planmodifier.List{listplanmodifier.UseStateForUnknown()},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Token id.",
						},
						"token": schema.StringAttribute{
							Computed:            true,
							Sensitive:           true,
							MarkdownDescription: "Token value.",
						},
					},
				},
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

// ModifyPlan rejects an unknown chain or network before anything is created,
// rather than letting the Admin API reject it partway through an apply.
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

	if label := plan.Label.ValueString(); label != "" {
		if err := r.client.SetEndpointLabel(ctx, created.ID, label); err != nil {
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

	tokens, tokenDiags := tokenList(endpoint.Tokens)
	diags.Append(tokenDiags...)
	model.Tokens = tokens
}

func applyEndpoint(endpoint *client.Endpoint, state *endpointResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	state.ID = types.StringValue(endpoint.ID)
	state.Chain = types.StringValue(endpoint.Chain)
	state.Network = types.StringValue(endpoint.Network)
	state.Status = types.StringValue(endpoint.Status)
	state.Multichain = types.BoolValue(endpoint.Multichain)
	applyEndpointURLs(endpoint, state)

	tokens, tokenDiags := tokenList(endpoint.Tokens)
	diags.Append(tokenDiags...)
	state.Tokens = tokens

	if endpoint.Label == "" {
		state.Label = types.StringNull()
	} else {
		state.Label = types.StringValue(endpoint.Label)
	}

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

// applyEndpointURLs maps empty URLs to null so that a chain without WebSocket
// support reports wss_url as absent rather than as an empty string.
func applyEndpointURLs(endpoint *client.Endpoint, model *endpointResourceModel) {
	model.HTTPURL = stringOrNull(endpoint.HTTPURL)
	model.WSSURL = stringOrNull(endpoint.WSSURL)
	model.HTTPURLWithToken = stringOrNull(endpoint.HTTPURLWithToken)
	model.WSSURLWithToken = stringOrNull(endpoint.WSSURLWithToken)
}

func stringOrNull(value string) types.String {
	if value == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}

func tokenList(tokens []client.EndpointToken) (types.List, diag.Diagnostics) {
	elementType := types.ObjectType{AttrTypes: endpointTokenAttrTypes}
	values := make([]attr.Value, 0, len(tokens))
	var diags diag.Diagnostics

	for _, token := range tokens {
		value, objectDiags := types.ObjectValue(endpointTokenAttrTypes, map[string]attr.Value{
			"id":    types.StringValue(token.ID),
			"token": types.StringValue(token.Value),
		})
		diags.Append(objectDiags...)
		values = append(values, value)
	}
	if diags.HasError() {
		return types.ListNull(elementType), diags
	}

	list, listDiags := types.ListValue(elementType, values)
	diags.Append(listDiags...)
	return list, diags
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
