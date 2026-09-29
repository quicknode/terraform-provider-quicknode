package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

var _ datasource.DataSourceWithConfigure = (*kvValueDataSource)(nil)

type kvValueDataSource struct {
	client *client.Client
}

type kvValueDataSourceModel struct {
	Key   types.String `tfsdk:"key"`
	Value types.String `tfsdk:"value"`
}

func NewKVValueDataSource() datasource.DataSource {
	return &kvValueDataSource{}
}

func (d *kvValueDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_kv_value"
}

func (d *kvValueDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A value in the Quicknode Key-Value Store. Reading a missing key is an error.",
		Attributes: map[string]schema.Attribute{
			"key": kvDataSourceKey("Name of the value."),
			"value": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The stored value.",
			},
		},
	}
}

func (d *kvValueDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if configured := kvDataSourceClient(req, resp, "key-value value"); configured != nil {
		d.client = configured
	}
}

func (d *kvValueDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config kvValueDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	value, err := d.client.GetKVValue(ctx, config.Key.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not read the key-value value", err.Error())
		return
	}
	config.Value = types.StringValue(value)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
