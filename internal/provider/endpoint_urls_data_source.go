package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

var _ datasource.DataSource = (*endpointURLsDataSource)(nil)
var _ datasource.DataSourceWithConfigure = (*endpointURLsDataSource)(nil)

type endpointURLsDataSource struct {
	client *client.Client
}

type endpointURLsDataSourceModel struct {
	EndpointID              types.String                `tfsdk:"endpoint_id"`
	SafeHTTPURL             types.String                `tfsdk:"safe_http_url"`
	SafeWSSURL              types.String                `tfsdk:"safe_wss_url"`
	HTTPURLWithToken        types.String                `tfsdk:"http_url_with_token"`
	WSSURLWithToken         types.String                `tfsdk:"wss_url_with_token"`
	SafeMultichainURLs      map[string]networkURLsModel `tfsdk:"safe_multichain_urls"`
	MultichainURLsWithToken map[string]networkURLsModel `tfsdk:"multichain_urls_with_token"`
}

type networkURLsModel struct {
	HTTPURL types.String `tfsdk:"http_url"`
	WSSURL  types.String `tfsdk:"wss_url"`
}

func NewEndpointURLsDataSource() datasource.DataSource {
	return &endpointURLsDataSource{}
}

func (d *endpointURLsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_endpoint_urls"
}

func (d *endpointURLsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	networkURLs := func(description string, sensitive bool) schema.MapNestedAttribute {
		return schema.MapNestedAttribute{
			Computed:            true,
			Sensitive:           sensitive,
			MarkdownDescription: description,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"http_url": schema.StringAttribute{Computed: true, MarkdownDescription: "The network's HTTPS URL."},
					"wss_url":  schema.StringAttribute{Computed: true, MarkdownDescription: "The network's WebSocket URL, or null on networks without WebSocket support."},
				},
			},
		}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "The URLs an endpoint serves, read fresh on every plan. The credentialed URLs carry whichever token the Admin API currently embeds, " +
			"which changes when tokens are added or removed, so they are read here and not tracked on `quicknode_endpoint`.\n\n" +
			"A multichain endpoint also serves every network in `safe_multichain_urls` and `multichain_urls_with_token`, keyed by network slug. " +
			"Both maps are empty when `multichain` is off.\n\n" +
			"Set `depends_on` to the endpoint, or to the `quicknode_endpoint_token` resources on it, when they are in the same configuration. " +
			"The id is known before the apply, so without it the URLs are read during the plan, before a change to `multichain` or to the tokens is applied.",
		Attributes: map[string]schema.Attribute{
			"endpoint_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Endpoint id.",
			},
			"safe_http_url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The HTTPS URL with the auth token replaced by `REPLACE_WITH_TOKEN`. Safe to log or display.",
			},
			"safe_wss_url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The WebSocket URL with the auth token replaced by `REPLACE_WITH_TOKEN`, or null on chains without WebSocket support.",
			},
			"http_url_with_token": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "The working HTTPS endpoint, exactly as the Admin API returns it.",
			},
			"wss_url_with_token": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "The working WebSocket endpoint, or null on chains without WebSocket support.",
			},
			"safe_multichain_urls":       networkURLs("Every network a multichain endpoint serves, with the auth token replaced by `REPLACE_WITH_TOKEN`. Safe to log or display.", false),
			"multichain_urls_with_token": networkURLs("Every network a multichain endpoint serves, with working URLs.", true),
		},
	}
}

func (d *endpointURLsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("The endpoint URLs data source expected providerData, got %T.", req.ProviderData))
		return
	}
	d.client = data.Client
}

func (d *endpointURLsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config endpointURLsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	urls, err := d.client.GetEndpointURLs(ctx, config.EndpointID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not read the endpoint's URLs", err.Error())
		return
	}

	config.SafeHTTPURL = stringOrNull(urls.SafeHTTPURL)
	config.SafeWSSURL = stringOrNull(urls.SafeWSSURL)
	config.HTTPURLWithToken = stringOrNull(urls.HTTPURLWithToken)
	config.WSSURLWithToken = stringOrNull(urls.WSSURLWithToken)

	config.SafeMultichainURLs = make(map[string]networkURLsModel, len(urls.Multichain))
	config.MultichainURLsWithToken = make(map[string]networkURLsModel, len(urls.Multichain))
	for network, networkURLs := range urls.Multichain {
		config.SafeMultichainURLs[network] = networkURLsModel{
			HTTPURL: stringOrNull(networkURLs.SafeHTTPURL),
			WSSURL:  stringOrNull(networkURLs.SafeWSSURL),
		}
		config.MultichainURLsWithToken[network] = networkURLsModel{
			HTTPURL: stringOrNull(networkURLs.HTTPURLWithToken),
			WSSURL:  stringOrNull(networkURLs.WSSURLWithToken),
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
