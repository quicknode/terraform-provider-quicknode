package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

var _ resource.Resource = (*kvValueResource)(nil)
var _ resource.ResourceWithConfigure = (*kvValueResource)(nil)
var _ resource.ResourceWithImportState = (*kvValueResource)(nil)

type kvValueResource struct {
	client *client.Client
}

type kvValueResourceModel struct {
	ID    types.String `tfsdk:"id"`
	Key   types.String `tfsdk:"key"`
	Value types.String `tfsdk:"value"`
}

func NewKVValueResource() resource.Resource {
	return &kvValueResource{}
}

func (r *kvValueResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_kv_value"
}

func (r *kvValueResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A value in the Quicknode Key-Value Store. Streams filters can read and update it, for example a threshold or the last block processed.\n\n" +
			"A filter that writes the value shows up as a change, and the next apply sets it back. Values and lists are separate, so a value and a list can share a key.",
		Attributes: map[string]schema.Attribute{
			"id":  kvIDAttribute("Same as `key`."),
			"key": kvKeyAttribute("Name of the value, as filters refer to it."),
			"value": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The value, as a string. Use `jsonencode` for structured data. Terraform state holds it in plain text.",
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
		},
	}
}

func (r *kvValueResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("The key-value value resource expected providerData, got %T.", req.ProviderData))
		return
	}
	r.client = data.Client
}

func (r *kvValueResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan kvValueResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	key := plan.Key.ValueString()

	_, err := r.client.GetKVValue(ctx, key)
	if err == nil {
		resp.Diagnostics.AddAttributeError(path.Root("key"), "Key-value value already exists",
			fmt.Sprintf("A value named %q already exists. Import it with `terraform import` to manage it, or choose another key.", key))
		return
	}
	if !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Could not check for an existing key-value value", err.Error())
		return
	}

	if err := r.client.SetKVValue(ctx, key, plan.Value.ValueString()); err != nil {
		resp.Diagnostics.AddError("Could not create the key-value value", err.Error())
		return
	}

	plan.ID = types.StringValue(key)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *kvValueResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state kvValueResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	value, err := r.client.GetKVValue(ctx, state.Key.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read the key-value value", err.Error())
		return
	}

	state.ID = state.Key
	state.Value = types.StringValue(value)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *kvValueResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state kvValueResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.SetKVValue(ctx, state.Key.ValueString(), plan.Value.ValueString()); err != nil {
		resp.Diagnostics.AddError("Could not update the key-value value", err.Error())
		return
	}

	plan.ID = state.ID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *kvValueResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state kvValueResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteKVValue(ctx, state.Key.ValueString()); err != nil {
		resp.Diagnostics.AddError("Could not delete the key-value value", err.Error())
	}
}

// ImportState takes the value's key.
func (r *kvValueResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("key"), req.ID)...)
}
