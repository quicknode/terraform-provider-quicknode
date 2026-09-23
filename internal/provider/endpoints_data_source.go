package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

var _ datasource.DataSource = (*endpointsDataSource)(nil)
var _ datasource.DataSourceWithConfigure = (*endpointsDataSource)(nil)

type endpointsDataSource struct {
	client *client.Client
}

type endpointsDataSourceModel struct {
	Search    types.String           `tfsdk:"search"`
	Networks  []types.String         `tfsdk:"networks"`
	Statuses  []types.String         `tfsdk:"statuses"`
	Labels    []types.String         `tfsdk:"labels"`
	TagLabels []types.String         `tfsdk:"tag_labels"`
	Endpoints []endpointSummaryModel `tfsdk:"endpoints"`
	IDs       []types.String         `tfsdk:"ids"`
}

type endpointSummaryModel struct {
	ID          types.String   `tfsdk:"id"`
	Name        types.String   `tfsdk:"name"`
	Label       types.String   `tfsdk:"label"`
	Chain       types.String   `tfsdk:"chain"`
	Network     types.String   `tfsdk:"network"`
	Status      types.String   `tfsdk:"status"`
	SafeHTTPURL types.String   `tfsdk:"safe_http_url"`
	SafeWSSURL  types.String   `tfsdk:"safe_wss_url"`
	Dedicated   types.Bool     `tfsdk:"dedicated"`
	FlatRate    types.Bool     `tfsdk:"flat_rate"`
	Multichain  types.Bool     `tfsdk:"multichain"`
	Tags        []types.String `tfsdk:"tags"`
}

func NewEndpointsDataSource() datasource.DataSource {
	return &endpointsDataSource{}
}

func (d *endpointsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_endpoints"
}

func (d *endpointsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	filter := func(description string) schema.ListAttribute {
		return schema.ListAttribute{
			Optional:            true,
			ElementType:         types.StringType,
			MarkdownDescription: description,
		}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Endpoints on the account, optionally filtered. Several values in one filter match any of them, and several filters must all match.\n\n" +
			"Rows carry what the list route returns, which is less than `data.quicknode_endpoint`: no tokens, no security settings and no rate limits. " +
			"Every page is walked, so the result covers the whole account.",
		Attributes: map[string]schema.Attribute{
			"search": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Match against the endpoint's subdomain or label.",
			},
			"networks":   filter("Keep only endpoints on these networks, for example `mainnet` or `base-sepolia`."),
			"statuses":   filter("Keep only endpoints with these statuses: `active` or `paused`."),
			"labels":     filter("Keep only endpoints carrying these labels."),
			"tag_labels": filter("Keep only endpoints carrying these tags."),
			"ids": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Ids of the matching endpoints, in the same order as `endpoints`. Convenient for `for_each` over another resource.",
			},
			"endpoints": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The matching endpoints.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":      schema.StringAttribute{Computed: true, MarkdownDescription: "Endpoint id."},
						"name":    schema.StringAttribute{Computed: true, MarkdownDescription: "Endpoint subdomain."},
						"label":   schema.StringAttribute{Computed: true, MarkdownDescription: "Descriptive label, or null if the endpoint has none."},
						"chain":   schema.StringAttribute{Computed: true, MarkdownDescription: "Chain slug."},
						"network": schema.StringAttribute{Computed: true, MarkdownDescription: "Network slug."},
						"status":  schema.StringAttribute{Computed: true, MarkdownDescription: "`active` or `paused`."},
						"safe_http_url": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The HTTPS URL with the auth token replaced by `TOKEN`. Safe to log or display. The list route carries no usable token, so read `data.quicknode_endpoint` for a working URL.",
						},
						"safe_wss_url": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The WebSocket URL with the auth token replaced by `TOKEN`, or null on chains without WebSocket support.",
						},
						"dedicated":  schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the endpoint runs on dedicated infrastructure."},
						"flat_rate":  schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the endpoint is billed at a flat rate."},
						"multichain": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the endpoint serves more than one network."},
						"tags": schema.ListAttribute{
							Computed:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Tag labels applied to the endpoint.",
						},
					},
				},
			},
		},
	}
}

func (d *endpointsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("The endpoints data source expected providerData, got %T.", req.ProviderData))
		return
	}
	d.client = data.Client
}

func (d *endpointsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config endpointsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoints, err := d.client.ListEndpoints(ctx, client.EndpointFilter{
		Search:    config.Search.ValueString(),
		Networks:  plainStrings(config.Networks),
		Statuses:  plainStrings(config.Statuses),
		Labels:    plainStrings(config.Labels),
		TagLabels: plainStrings(config.TagLabels),
	})
	if err != nil {
		resp.Diagnostics.AddError("Could not list the Quicknode endpoints", err.Error())
		return
	}

	config.Endpoints = make([]endpointSummaryModel, 0, len(endpoints))
	config.IDs = make([]types.String, 0, len(endpoints))
	for _, endpoint := range endpoints {
		tags := make([]types.String, 0, len(endpoint.Tags))
		for _, tag := range endpoint.Tags {
			tags = append(tags, types.StringValue(tag.Label))
		}
		config.Endpoints = append(config.Endpoints, endpointSummaryModel{
			ID:          types.StringValue(endpoint.ID),
			Name:        types.StringValue(endpoint.Name),
			Label:       stringOrNull(endpoint.Label),
			Chain:       types.StringValue(endpoint.Chain),
			Network:     types.StringValue(endpoint.Network),
			Status:      types.StringValue(endpoint.Status),
			SafeHTTPURL: stringOrNull(endpoint.SafeHTTPURL),
			SafeWSSURL:  stringOrNull(endpoint.SafeWSSURL),
			Dedicated:   types.BoolValue(endpoint.Dedicated),
			FlatRate:    types.BoolValue(endpoint.FlatRate),
			Multichain:  types.BoolValue(endpoint.Multichain),
			Tags:        tags,
		})
		config.IDs = append(config.IDs, types.StringValue(endpoint.ID))
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func plainStrings(values []types.String) []string {
	if len(values) == 0 {
		return nil
	}
	plain := make([]string, 0, len(values))
	for _, value := range values {
		if value.IsNull() || value.IsUnknown() {
			continue
		}
		plain = append(plain, value.ValueString())
	}
	return plain
}
