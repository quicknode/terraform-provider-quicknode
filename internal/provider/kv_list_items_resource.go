package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

var _ resource.Resource = (*kvListItemsResource)(nil)
var _ resource.ResourceWithConfigure = (*kvListItemsResource)(nil)
var _ resource.ResourceWithImportState = (*kvListItemsResource)(nil)

const kvListItemsConflictPreview = 5

type kvListItemsResource struct {
	client *client.Client
}

type kvListItemsResourceModel struct {
	ID      types.String `tfsdk:"id"`
	ListKey types.String `tfsdk:"list_key"`
	Items   types.Set    `tfsdk:"items"`
}

func NewKVListItemsResource() resource.Resource {
	return &kvListItemsResource{}
}

func (r *kvListItemsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_kv_list_items"
}

func (r *kvListItemsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Items in a Quicknode Key-Value Store list. Terraform manages only these items and leaves the rest of the list alone, " +
			"for lists that Streams filters or other tools also write to. Adding items to a missing list creates it.\n\n" +
			"Create fails if any of the items are already in the list, so destroy never removes items Terraform didn't add. " +
			"To take over existing items, import the resource and apply. " +
			"Don't use it with a `quicknode_kv_list` for the same key, since that resource removes items it doesn't know about.",
		Attributes: map[string]schema.Attribute{
			"id":       kvIDAttribute("Same as `list_key`."),
			"list_key": kvKeyAttribute("Name of the list the items belong to."),
			"items": schema.SetAttribute{
				Required:    true,
				ElementType: types.StringType,
				MarkdownDescription: "Items this resource adds to the list. Items are case-sensitive, so `0xABC` and `0xabc` are different items. " +
					"Changes are sent as additions and removals, up to 1500 items per request. Items removed outside Terraform are added back on the next apply.",
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},
		},
	}
}

func (r *kvListItemsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("The key-value list items resource expected providerData, got %T.", req.ProviderData))
		return
	}
	r.client = data.Client
}

func (r *kvListItemsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan kvListItemsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	listKey := plan.ListKey.ValueString()

	items, diags := setStrings(ctx, plan.Items)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	existing, err := r.client.GetKVList(ctx, listKey)
	if err != nil {
		resp.Diagnostics.AddError("Could not read the key-value list", err.Error())
		return
	}
	if conflicts := setIntersection(items, existing); len(conflicts) > 0 {
		preview := conflicts[:min(len(conflicts), kvListItemsConflictPreview)]
		resp.Diagnostics.AddAttributeError(path.Root("items"), "Items are already in the list",
			fmt.Sprintf("List %q already holds %d of these items, including %s. To manage them, import this resource with the list key %q and apply.",
				listKey, len(conflicts), strings.Join(quoteAll(preview), ", "), listKey))
		return
	}

	if err := r.client.UpdateKVList(ctx, listKey, items, nil); err != nil {
		resp.Diagnostics.AddError("Could not add the items to the key-value list", err.Error())
		return
	}

	plan.ID = types.StringValue(listKey)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read keeps only the items this resource added that are still in the list,
// so items other writers add never show up as a change.
func (r *kvListItemsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state kvListItemsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	managed, diags := setStrings(ctx, state.Items)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	existing, err := r.client.GetKVList(ctx, state.ListKey.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not read the key-value list", err.Error())
		return
	}

	set, diags := stringSet(setIntersection(managed, existing))
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.ID = state.ListKey
	state.Items = set
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *kvListItemsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state kvListItemsResourceModel
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
	if err := r.client.UpdateKVList(ctx, state.ListKey.ValueString(), add, remove); err != nil {
		resp.Diagnostics.AddError("Could not update the key-value list", err.Error())
		return
	}

	plan.ID = state.ID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *kvListItemsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state kvListItemsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	items, diags := setStrings(ctx, state.Items)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateKVList(ctx, state.ListKey.ValueString(), nil, items); err != nil {
		resp.Diagnostics.AddError("Could not remove the items from the key-value list", err.Error())
	}
}

// ImportState takes the list key and starts with no items, since the list
// alone cannot say which items this resource should own. The next apply takes
// over the configured items, including any that are already in the list.
func (r *kvListItemsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("list_key"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("items"), types.SetValueMust(types.StringType, nil))...)
}

func quoteAll(items []string) []string {
	quoted := make([]string, len(items))
	for index, item := range items {
		quoted[index] = fmt.Sprintf("%q", item)
	}
	return quoted
}
