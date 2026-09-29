package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

var _ datasource.DataSourceWithConfigure = (*kvListDataSource)(nil)

type kvListDataSource struct {
	client *client.Client
}

type kvListDataSourceModel struct {
	Key   types.String `tfsdk:"key"`
	Items types.Set    `tfsdk:"items"`
}

func NewKVListDataSource() datasource.DataSource {
	return &kvListDataSource{}
}

func (d *kvListDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_kv_list"
}

func (d *kvListDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Items in a Quicknode Key-Value Store list. A missing list reads as empty.",
		Attributes: map[string]schema.Attribute{
			"key": kvDataSourceKey("Name of the list."),
			"items": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Items in the list.",
			},
		},
	}
}

func (d *kvListDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if configured := kvDataSourceClient(req, resp, "key-value list"); configured != nil {
		d.client = configured
	}
}

func (d *kvListDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config kvListDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	items, err := d.client.GetKVList(ctx, config.Key.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not read the key-value list", err.Error())
		return
	}

	set, diags := stringSet(items)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	config.Items = set
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
