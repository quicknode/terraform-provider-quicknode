package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

var _ datasource.DataSource = (*streamDataSource)(nil)
var _ datasource.DataSourceWithConfigure = (*streamDataSource)(nil)

var streamSecretAttributes = map[string]bool{
	"security_token": true,
	"secret_key":     true,
	"sas_token":      true,
	"password":       true,
	"ssl_key_pem":    true,
}

type streamDataSource struct {
	client *client.Client
}

type streamDataSourceModel struct {
	ID                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	Network              types.String `tfsdk:"network"`
	Dataset              types.String `tfsdk:"dataset"`
	Region               types.String `tfsdk:"region"`
	Status               types.String `tfsdk:"status"`
	FilterFunction       types.String `tfsdk:"filter_function"`
	FilterLanguage       types.String `tfsdk:"filter_language"`
	StartRange           types.Int64  `tfsdk:"start_range"`
	EndRange             types.Int64  `tfsdk:"end_range"`
	Sequence             types.Int64  `tfsdk:"sequence"`
	DatasetBatchSize     types.Int64  `tfsdk:"dataset_batch_size"`
	ElasticBatchEnabled  types.Bool   `tfsdk:"elastic_batch_enabled"`
	FixBlockReorgs       types.Bool   `tfsdk:"fix_block_reorgs"`
	KeepDistanceFromTip  types.Int64  `tfsdk:"keep_distance_from_tip"`
	RestreamBatchOnReorg types.Bool   `tfsdk:"restream_batch_on_reorg"`
	NotificationEmail    types.String `tfsdk:"notification_email"`
	Destination          types.Object `tfsdk:"destination"`
	ExtraDestinations    types.List   `tfsdk:"extra_destinations"`
}

func NewStreamDataSource() datasource.DataSource {
	return &streamDataSource{}
}

func (d *streamDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_stream"
}

func (d *streamDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	computedString := func(description string) schema.StringAttribute {
		return schema.StringAttribute{Computed: true, MarkdownDescription: description}
	}
	computedInt := func(description string) schema.Int64Attribute {
		return schema.Int64Attribute{Computed: true, MarkdownDescription: description}
	}
	computedBool := func(description string) schema.BoolAttribute {
		return schema.BoolAttribute{Computed: true, MarkdownDescription: description}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "An existing stream, looked up by `id`. " +
			"Destination credentials are stored in Terraform state, so keep state encrypted and remote.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Stream id.",
			},
			"name":                    computedString("Name shown in the dashboard."),
			"network":                 computedString("Network slug."),
			"dataset":                 computedString("What each batch carries."),
			"region":                  computedString("Region the stream runs in."),
			"status":                  computedString("`active`, `paused`, `terminated` or `completed`."),
			"filter_function":         computedString("Source code of the filter, or null if the stream has none."),
			"filter_language":         computedString("`javascript` or `go`."),
			"start_range":             computedInt("First block the stream delivers."),
			"end_range":               computedInt("Last block the stream delivers, or null if it runs until paused."),
			"sequence":                computedInt("Last block delivered. It moves while the stream runs."),
			"dataset_batch_size":      computedInt("Blocks per batch."),
			"elastic_batch_enabled":   computedBool("Whether batches grow while the stream catches up."),
			"fix_block_reorgs":        computedBool("Whether corrected blocks are delivered when the network reorganizes."),
			"keep_distance_from_tip":  computedInt("How many blocks the stream stays behind the newest block."),
			"restream_batch_on_reorg": computedBool("Whether the whole batch is delivered again when a block in it is reorganized."),
			"notification_email":      computedString("Address notified when the stream is terminated, or null."),
			"destination": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Where the stream delivers. One type is set; the others are null.",
				Attributes:          computedDestinationAttributes(),
			},
			"extra_destinations": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "More destinations that receive every batch, or null if there are none.",
				NestedObject:        schema.NestedAttributeObject{Attributes: computedDestinationAttributes()},
			},
		},
	}
}

// computedDestinationAttributes mirrors the resource's destination types as
// read-only attributes, built from the same attribute types so the two cannot
// drift apart.
func computedDestinationAttributes() map[string]schema.Attribute {
	attributes := map[string]schema.Attribute{}
	for name, destinationType := range streamDestinationAttrTypes {
		children := map[string]schema.Attribute{}
		for childName, childType := range destinationType.(types.ObjectType).AttrTypes {
			description := fmt.Sprintf("See `destination.%s.%s` on the `quicknode_stream` resource.", name, childName)
			children[childName] = computedAttribute(childType, streamSecretAttributes[childName], description)
		}
		attributes[name] = schema.SingleNestedAttribute{
			Computed:            true,
			MarkdownDescription: fmt.Sprintf("Settings when the destination is `%s`, otherwise null.", name),
			Attributes:          children,
		}
	}
	return attributes
}

func computedAttribute(attrType attr.Type, sensitive bool, description string) schema.Attribute {
	switch attrType {
	case types.Int64Type:
		return schema.Int64Attribute{Computed: true, MarkdownDescription: description}
	case types.BoolType:
		return schema.BoolAttribute{Computed: true, MarkdownDescription: description}
	case types.StringType:
		return schema.StringAttribute{Computed: true, Sensitive: sensitive, MarkdownDescription: description}
	default:
		return schema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: description}
	}
}

func (d *streamDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("The stream data source expected providerData, got %T.", req.ProviderData))
		return
	}
	d.client = data.Client
}

func (d *streamDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config streamDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	stream, err := d.client.GetStream(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not read the Quicknode stream", err.Error())
		return
	}

	config.Name = types.StringValue(stream.Name)
	config.Network = types.StringValue(stream.Network)
	config.Dataset = types.StringValue(stream.Dataset)
	config.Region = types.StringValue(stream.Region)
	config.Status = types.StringValue(stream.Status)
	config.FilterFunction = stringOrNull(stream.FilterFunction)
	config.FilterLanguage = types.StringValue(stream.FilterLanguage)
	config.StartRange = types.Int64Value(stream.StartRange)
	config.EndRange = rangeOrNull(stream.EndRange)
	config.Sequence = types.Int64Value(stream.Sequence)
	config.DatasetBatchSize = types.Int64Value(stream.DatasetBatchSize)
	config.ElasticBatchEnabled = types.BoolValue(stream.ElasticBatchEnabled)
	config.FixBlockReorgs = types.BoolValue(stream.FixBlockReorgs != 0)
	config.KeepDistanceFromTip = types.Int64Value(stream.KeepDistanceFromTip)
	config.RestreamBatchOnReorg = types.BoolValue(stream.RestreamBatchOnReorg)
	config.NotificationEmail = stringOrNull(stream.NotificationEmail)

	destination, diags := destinationObject(stream.Destination)
	resp.Diagnostics.Append(diags...)
	config.Destination = destination

	extras, diags := destinationsList(stream.ExtraDestinations)
	resp.Diagnostics.Append(diags...)
	config.ExtraDestinations = extras

	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
