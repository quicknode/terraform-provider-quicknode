package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

var _ resource.Resource = (*endpointTokenResource)(nil)
var _ resource.ResourceWithConfigure = (*endpointTokenResource)(nil)
var _ resource.ResourceWithImportState = (*endpointTokenResource)(nil)

type endpointTokenResource struct {
	client *client.Client
}

type endpointTokenResourceModel struct {
	ID               types.String `tfsdk:"id"`
	EndpointID       types.String `tfsdk:"endpoint_id"`
	Token            types.String `tfsdk:"token"`
	HTTPURLWithToken types.String `tfsdk:"http_url_with_token"`
	WSSURLWithToken  types.String `tfsdk:"wss_url_with_token"`
}

func NewEndpointTokenResource() resource.Resource {
	return &endpointTokenResource{}
}

func (r *endpointTokenResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_endpoint_token"
}

func (r *endpointTokenResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An additional auth token on a Quicknode endpoint. Every endpoint is created with one token already; this resource adds further tokens, so a credential can be handed to one consumer and later revoked without disturbing the others.\n\n" +
			"Quicknode generates the value, so the resource takes no input beyond the endpoint. To rotate a token, add the replacement, move consumers across, then remove the old resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Token id assigned by Quicknode.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"endpoint_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Endpoint the token belongs to.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"token": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "The token value. It is stored in Terraform state, so keep state encrypted and remote.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"http_url_with_token": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "The endpoint's HTTPS URL carrying this token. Pass it to the consumer the token was issued for.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"wss_url_with_token": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "The endpoint's WebSocket URL carrying this token, or null on chains without WebSocket support.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *endpointTokenResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("The endpoint token resource expected providerData, got %T.", req.ProviderData))
		return
	}
	r.client = data.Client
}

func (r *endpointTokenResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan endpointTokenResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.AddEndpointToken(ctx, plan.EndpointID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not add the endpoint token", err.Error())
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.Token = types.StringValue(created.Value)
	resp.Diagnostics.Append(r.applyURLs(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	resp.Diagnostics.Append(warnToggleDisabled(ctx, r.client, plan.EndpointID.ValueString(), "tokens", "Token authentication")...)
}

func (r *endpointTokenResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state endpointTokenResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	security, err := r.client.GetEndpointSecurity(ctx, state.EndpointID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read the endpoint's tokens", err.Error())
		return
	}

	for _, token := range security.Tokens {
		if token.ID != state.ID.ValueString() {
			continue
		}
		state.Token = types.StringValue(token.Value)
		if state.HTTPURLWithToken.IsNull() {
			resp.Diagnostics.Append(r.applyURLs(ctx, &state)...)
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}
	// The security route omits a list entirely while its toggle is disabled,
	// so an entry that cannot be seen has not necessarily been deleted.
	// Dropping it from state here would have the next apply create a duplicate.
	if !securityToggleEnabled(security.Options, "tokens") {
		return
	}
	resp.State.RemoveResource(ctx)
}

// Update never runs: the endpoint replaces the resource and Quicknode generates the value.
func (r *endpointTokenResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}

func (r *endpointTokenResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state endpointTokenResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.RemoveEndpointToken(ctx, state.EndpointID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Could not remove the endpoint token", err.Error())
	}
}

// ImportState takes "<endpoint id>/<token id>". Addressing a token by id keeps
// the credential off the command line and out of shell history.
func (r *endpointTokenResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	endpointID, tokenID, found := strings.Cut(req.ID, "/")
	if !found || endpointID == "" || tokenID == "" {
		resp.Diagnostics.AddError(
			"Unexpected import address",
			fmt.Sprintf("Import an endpoint token as \"<endpoint id>/<token id>\", for example \"123456/e5d4c3b2-...\". Got %q.", req.ID),
		)
		return
	}

	security, err := r.client.GetEndpointSecurity(ctx, endpointID)
	if err != nil {
		resp.Diagnostics.AddError("Could not read the endpoint's tokens", err.Error())
		return
	}

	for _, token := range security.Tokens {
		if token.ID != tokenID {
			continue
		}
		state := endpointTokenResourceModel{
			ID:         types.StringValue(token.ID),
			EndpointID: types.StringValue(endpointID),
			Token:      types.StringValue(token.Value),
		}
		resp.Diagnostics.Append(r.applyURLs(ctx, &state)...)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}
	resp.Diagnostics.AddError("No matching token", fmt.Sprintf("Endpoint %s has no token with the id %q.", endpointID, tokenID))
}

// applyURLs builds the token's URLs from the endpoint's redacted ones, so they
// carry this token whichever one the Admin API embeds in its own URLs.
func (r *endpointTokenResource) applyURLs(ctx context.Context, model *endpointTokenResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	endpoint, err := r.client.GetEndpoint(ctx, model.EndpointID.ValueString())
	if err != nil {
		diags.AddError("Could not read the endpoint's URLs", err.Error())
		return diags
	}
	token := model.Token.ValueString()
	model.HTTPURLWithToken = stringOrNull(client.EndpointURLWithToken(endpoint.SafeHTTPURL, token))
	model.WSSURLWithToken = stringOrNull(client.EndpointURLWithToken(endpoint.SafeWSSURL, token))
	return diags
}
