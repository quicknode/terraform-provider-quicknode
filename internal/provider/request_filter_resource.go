package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

var _ resource.Resource = (*requestFilterResource)(nil)
var _ resource.ResourceWithConfigure = (*requestFilterResource)(nil)
var _ resource.ResourceWithImportState = (*requestFilterResource)(nil)

type requestFilterResource struct {
	client *client.Client
}

type requestFilterResourceModel struct {
	ID         types.String `tfsdk:"id"`
	EndpointID types.String `tfsdk:"endpoint_id"`
	Methods    types.Set    `tfsdk:"methods"`
}

func NewRequestFilterResource() resource.Resource {
	return &requestFilterResource{}
}

func (r *requestFilterResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_endpoint_request_filter"
}

func (r *requestFilterResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The set of RPC methods a Quicknode endpoint accepts. Anything outside the set is rejected, which keeps an endpoint handed to a browser or a third party from reaching methods it has no reason to call.\n\n" +
			"`security_options.request_filters` on the endpoint reports whether filtering is applied. It is read-only: the Admin API turns it on when a filter exists and off when the last one is removed.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Filter id assigned by Quicknode.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"endpoint_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Endpoint the filter belongs to.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"methods": schema.SetAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "RPC methods the endpoint accepts, for example `eth_call` and `eth_getLogs`. Editing the set updates the filter in place.",
				Validators:          []validator.Set{setvalidator.SizeAtLeast(1)},
			},
		},
	}
}

func (r *requestFilterResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("The request filter resource expected providerData, got %T.", req.ProviderData))
		return
	}
	r.client = data.Client
}

func (r *requestFilterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan requestFilterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	methods, diags := methodNames(ctx, plan.Methods)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.AddRequestFilter(ctx, plan.EndpointID.ValueString(), methods)
	if err != nil {
		resp.Diagnostics.AddError("Could not create the request filter", err.Error())
		return
	}

	plan.ID = types.StringValue(created.ID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *requestFilterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state requestFilterResourceModel
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
		resp.Diagnostics.AddError("Could not read the endpoint's request filters", err.Error())
		return
	}

	for _, filter := range security.RequestFilters {
		if filter.ID != state.ID.ValueString() {
			continue
		}
		methods, diags := methodSet(filter.Methods)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		state.Methods = methods
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}
	resp.State.RemoveResource(ctx)
}

func (r *requestFilterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state requestFilterResourceModel
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
	if err := r.client.UpdateRequestFilter(ctx, state.EndpointID.ValueString(), state.ID.ValueString(), methods); err != nil {
		resp.Diagnostics.AddError("Could not update the request filter", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *requestFilterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state requestFilterResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.RemoveRequestFilter(ctx, state.EndpointID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Could not remove the request filter", err.Error())
	}
}

// ImportState takes "<endpoint id>/<filter id>". A filter has no natural name,
// so unlike the allowlist entries it is addressed by its id. Read them from
// the endpoint's security settings in the Quicknode dashboard or from
// `GET /v0/endpoints/{id}/security`.
func (r *requestFilterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	endpointID, filterID, found := strings.Cut(req.ID, "/")
	if !found || endpointID == "" || filterID == "" {
		resp.Diagnostics.AddError(
			"Unexpected import address",
			fmt.Sprintf("Import a request filter as \"<endpoint id>/<filter id>\", for example \"123456/f1e2d3c4-...\". Got %q.", req.ID),
		)
		return
	}

	security, err := r.client.GetEndpointSecurity(ctx, endpointID)
	if err != nil {
		resp.Diagnostics.AddError("Could not read the endpoint's request filters", err.Error())
		return
	}

	for _, filter := range security.RequestFilters {
		if filter.ID != filterID {
			continue
		}
		methods, diags := methodSet(filter.Methods)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		state := requestFilterResourceModel{
			ID:         types.StringValue(filter.ID),
			EndpointID: types.StringValue(endpointID),
			Methods:    methods,
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}
	resp.Diagnostics.AddError("No matching request filter", fmt.Sprintf("Endpoint %s has no request filter with the id %q.", endpointID, filterID))
}

func methodNames(ctx context.Context, methods types.Set) ([]string, diag.Diagnostics) {
	var names []string
	diags := methods.ElementsAs(ctx, &names, false)
	return names, diags
}

func methodSet(methods []string) (types.Set, diag.Diagnostics) {
	values := make([]attr.Value, 0, len(methods))
	for _, method := range methods {
		values = append(values, types.StringValue(method))
	}
	return types.SetValue(types.StringType, values)
}
