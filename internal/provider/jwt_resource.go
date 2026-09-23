package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

var _ resource.Resource = (*jwtResource)(nil)
var _ resource.ResourceWithConfigure = (*jwtResource)(nil)
var _ resource.ResourceWithImportState = (*jwtResource)(nil)

type jwtResource struct {
	client *client.Client
}

type jwtResourceModel struct {
	ID         types.String `tfsdk:"id"`
	EndpointID types.String `tfsdk:"endpoint_id"`
	Name       types.String `tfsdk:"name"`
	PublicKey  types.String `tfsdk:"public_key"`
	KID        types.String `tfsdk:"kid"`
}

func NewJWTResource() resource.Resource {
	return &jwtResource{}
}

func (r *jwtResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_endpoint_jwt"
}

func (r *jwtResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A JWT signing key registered on a Quicknode endpoint. Callers then authenticate with a token signed by the matching private key, which keeps a long-lived credential out of the URL.\n\n" +
			"The key only takes effect once `security_options.jwts` is enabled on the endpoint.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Entry id assigned by Quicknode.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"endpoint_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Endpoint the signing key belongs to.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name for the key, used to tell several keys apart on one endpoint.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"public_key": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "PEM-encoded public key that signed tokens are verified against. Changing it replaces the entry, because the Admin API has no route to edit one in place.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"kid": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Key id for this signing key. Put the same value in the `kid` header of the tokens signed with the matching private key. Changing it replaces the entry, because the Admin API has no route to edit one in place.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
		},
	}
}

func (r *jwtResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("The JWT resource expected providerData, got %T.", req.ProviderData))
		return
	}
	r.client = data.Client
}

func (r *jwtResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan jwtResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.AddJWT(ctx, plan.EndpointID.ValueString(), client.JWT{
		Name:      plan.Name.ValueString(),
		KID:       plan.KID.ValueString(),
		PublicKey: plan.PublicKey.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Could not register the JWT signing key", err.Error())
		return
	}

	plan.ID = types.StringValue(created.ID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	resp.Diagnostics.Append(warnToggleDisabled(ctx, r.client, plan.EndpointID.ValueString(), "jwts", "JWT authentication")...)
}

func (r *jwtResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state jwtResourceModel
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
		resp.Diagnostics.AddError("Could not read the endpoint's JWT signing keys", err.Error())
		return
	}

	for _, jwt := range security.JWTs {
		if jwt.ID != state.ID.ValueString() {
			continue
		}
		state.Name = types.StringValue(jwt.Name)
		state.KID = types.StringValue(jwt.KID)
		if jwt.PublicKey != "" {
			state.PublicKey = types.StringValue(jwt.PublicKey)
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}
	// The security route omits a list entirely while its toggle is disabled,
	// so an entry that cannot be seen has not necessarily been deleted.
	// Dropping it from state here would have the next apply create a duplicate.
	if !securityToggleEnabled(security.Options, "jwts") {
		return
	}
	resp.State.RemoveResource(ctx)
}

// Update never runs: every attribute replaces the resource.
func (r *jwtResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}

func (r *jwtResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state jwtResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.RemoveJWT(ctx, state.EndpointID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Could not remove the JWT signing key", err.Error())
	}
}

// ImportState takes "<endpoint id>/<name>".
func (r *jwtResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	endpointID, name, found := strings.Cut(req.ID, "/")
	if !found || endpointID == "" || name == "" {
		resp.Diagnostics.AddError(
			"Unexpected import address",
			fmt.Sprintf("Import a JWT signing key as \"<endpoint id>/<name>\", for example \"652052/signer\". Got %q.", req.ID),
		)
		return
	}

	security, err := r.client.GetEndpointSecurity(ctx, endpointID)
	if err != nil {
		resp.Diagnostics.AddError("Could not read the endpoint's JWT signing keys", err.Error())
		return
	}

	matches := make([]client.JWT, 0, 1)
	for _, jwt := range security.JWTs {
		if jwt.Name == name {
			matches = append(matches, jwt)
		}
	}
	switch len(matches) {
	case 0:
		resp.Diagnostics.AddError("No matching JWT signing key", fmt.Sprintf("Endpoint %s has no JWT signing key named %q.", endpointID, name))
		return
	case 1:
	default:
		resp.Diagnostics.AddError(
			"More than one matching JWT signing key",
			fmt.Sprintf("Endpoint %s has %d JWT signing keys named %q, so this address is ambiguous.", endpointID, len(matches), name),
		)
		return
	}

	state := jwtResourceModel{
		ID:         types.StringValue(matches[0].ID),
		EndpointID: types.StringValue(endpointID),
		Name:       types.StringValue(matches[0].Name),
		PublicKey:  types.StringValue(matches[0].PublicKey),
		KID:        types.StringValue(matches[0].KID),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
