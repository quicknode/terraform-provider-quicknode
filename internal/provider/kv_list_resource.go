package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

var _ resource.Resource = (*kvListResource)(nil)
var _ resource.ResourceWithConfigure = (*kvListResource)(nil)
var _ resource.ResourceWithImportState = (*kvListResource)(nil)

type kvListResource struct {
	client *client.Client
}

type kvListResourceModel struct {
	ID    types.String `tfsdk:"id"`
	Key   types.String `tfsdk:"key"`
	Items types.Set    `tfsdk:"items"`
}

func NewKVListResource() resource.Resource {
	return &kvListResource{}
}

func (r *kvListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_kv_list"
}

func (r *kvListResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A list in the Quicknode Key-Value Store. Streams filters can read and update it, for example a set of wallet addresses to watch.\n\n" +
			"This resource owns the whole list: items added outside Terraform are removed on the next apply. " +
			"For a list that a filter or another tool also writes to, use `quicknode_kv_list_items`. Don't manage the same list with both.",
		Attributes: map[string]schema.Attribute{
			"id":  kvIDAttribute("Same as `key`."),
			"key": kvKeyAttribute("Name of the list, as filters refer to it."),
			"items": schema.SetAttribute{
				Required:    true,
				ElementType: types.StringType,
				MarkdownDescription: "Items in the list. Items are case-sensitive, so `0xABC` and `0xabc` are different items. " +
					"Changes are sent as additions and removals, up to 1500 items per request.",
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},
		},
	}
}

func (r *kvListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("The key-value list resource expected providerData, got %T.", req.ProviderData))
		return
	}
	r.client = data.Client
}

func (r *kvListResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan kvListResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	key := plan.Key.ValueString()

	items, diags := setStrings(ctx, plan.Items)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	existing, err := r.client.GetKVList(ctx, key)
	if err != nil {
		resp.Diagnostics.AddError("Could not check for an existing key-value list", err.Error())
		return
	}
	if len(existing) > 0 {
		resp.Diagnostics.AddAttributeError(path.Root("key"), "Key-value list already exists",
			fmt.Sprintf("A list named %q already exists with %d items. Import it with `terraform import` to manage it, or choose another key.", key, len(existing)))
		return
	}

	if err := r.client.CreateKVList(ctx, key, items); err != nil {
		resp.Diagnostics.AddError("Could not create the key-value list", err.Error())
		return
	}

	plan.ID = types.StringValue(key)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *kvListResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state kvListResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	items, err := r.client.GetKVList(ctx, state.Key.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not read the key-value list", err.Error())
		return
	}
	if len(items) == 0 {
		resp.State.RemoveResource(ctx)
		return
	}

	set, diags := stringSet(items)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.ID = state.Key
	state.Items = set
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *kvListResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state kvListResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	planned, diags := setStrings(ctx, plan.Items)
	resp.Diagnostics.Append(diags...)
	current, diags := setStrings(ctx, state.Items)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	add := setDifference(planned, current)
	remove := setDifference(current, planned)
	if err := r.client.UpdateKVList(ctx, state.Key.ValueString(), add, remove); err != nil {
		resp.Diagnostics.AddError("Could not update the key-value list", err.Error())
		return
	}

	plan.ID = state.ID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *kvListResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state kvListResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteKVList(ctx, state.Key.ValueString()); err != nil {
		resp.Diagnostics.AddError("Could not delete the key-value list", err.Error())
	}
}

// ImportState takes the list's key.
func (r *kvListResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("key"), req.ID)...)
}
