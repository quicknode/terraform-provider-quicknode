package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

var _ datasource.DataSource = (*endpointDataSource)(nil)
var _ datasource.DataSourceWithConfigure = (*endpointDataSource)(nil)
var _ datasource.DataSourceWithConfigValidators = (*endpointDataSource)(nil)

type endpointDataSource struct {
	client *client.Client
}

type endpointDataSourceModel struct {
	ID               types.String   `tfsdk:"id"`
	Label            types.String   `tfsdk:"label"`
	Chain            types.String   `tfsdk:"chain"`
	Network          types.String   `tfsdk:"network"`
	Status           types.String   `tfsdk:"status"`
	Multichain       types.Bool     `tfsdk:"multichain"`
	Tags             []types.String `tfsdk:"tags"`
	HTTPURL          types.String   `tfsdk:"http_url"`
	WSSURL           types.String   `tfsdk:"wss_url"`
	HTTPURLWithToken types.String   `tfsdk:"http_url_with_token"`
	WSSURLWithToken  types.String   `tfsdk:"wss_url_with_token"`
	Tokens           []tokenModel   `tfsdk:"tokens"`
	SecurityOptions  types.Object   `tfsdk:"security_options"`
	IPCustomHeader   types.String   `tfsdk:"ip_custom_header"`
}

type tokenModel struct {
	ID    types.String `tfsdk:"id"`
	Token types.String `tfsdk:"token"`
}

func NewEndpointDataSource() datasource.DataSource {
	return &endpointDataSource{}
}

func (d *endpointDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_endpoint"
}

func (d *endpointDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("label")),
	}
}

func (d *endpointDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	computedBool := func(description string) schema.BoolAttribute {
		return schema.BoolAttribute{Computed: true, MarkdownDescription: description}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "One endpoint that already exists on the account, looked up by `id` or by `label`. Use it to wire a Terraform configuration into an endpoint created elsewhere without importing it.\n\n" +
			"Labels are not unique, so a label matching more than one endpoint is an error rather than an arbitrary pick.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Endpoint id. Set this or `label`, not both.",
			},
			"label": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Endpoint label. Set this or `id`, not both.",
			},
			"chain":      schema.StringAttribute{Computed: true, MarkdownDescription: "Chain slug."},
			"network":    schema.StringAttribute{Computed: true, MarkdownDescription: "Network slug."},
			"status":     schema.StringAttribute{Computed: true, MarkdownDescription: "`active` or `paused`."},
			"multichain": computedBool("Whether the endpoint serves more than one network."),
			"tags": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Tag labels applied to the endpoint.",
			},
			"http_url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "HTTPS URL with the auth token removed. Safe to expose, but not a working endpoint.",
			},
			"wss_url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "WebSocket URL with the auth token removed, or null on chains without WebSocket support.",
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
			"ip_custom_header": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Header the endpoint reads the caller's IP address from, or null if none is set.",
			},
			"tokens": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Auth tokens for the endpoint. Values land in Terraform state, so keep state encrypted and remote.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":    schema.StringAttribute{Computed: true, MarkdownDescription: "Token id."},
						"token": schema.StringAttribute{Computed: true, Sensitive: true, MarkdownDescription: "Token value."},
					},
				},
			},
			"security_options": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Which security mechanisms the endpoint enforces.",
				Attributes: map[string]schema.Attribute{
					"tokens":           computedBool("Whether an auth token is required."),
					"referrers":        computedBool("Whether referrer restrictions are applied."),
					"jwts":             computedBool("Whether a signed JWT is required."),
					"ips":              computedBool("Whether IP restrictions are applied."),
					"domain_masks":     computedBool("Whether the endpoint is served from an approved custom domain."),
					"hsts":             computedBool("Whether the HTTP Strict Transport Security header is sent."),
					"cors":             computedBool("Whether a Cross-Origin Resource Sharing policy is applied."),
					"request_filters":  computedBool("Whether RPC method filtering is applied."),
					"response_logging": computedBool("Whether responses are logged for the endpoint."),
				},
			},
		},
	}
}

func (d *endpointDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("The endpoint data source expected providerData, got %T.", req.ProviderData))
		return
	}
	d.client = data.Client
}

func (d *endpointDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config endpointDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpointID := config.ID.ValueString()
	if endpointID == "" {
		match, err := d.client.FindEndpointByLabel(ctx, config.Label.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Could not find an endpoint with that label", err.Error())
			return
		}
		endpointID = match.ID
	}

	endpoint, err := d.client.GetEndpoint(ctx, endpointID)
	if err != nil {
		resp.Diagnostics.AddError("Could not read the Quicknode endpoint", err.Error())
		return
	}

	config.ID = types.StringValue(endpoint.ID)
	config.Label = stringOrNull(endpoint.Label)
	config.Chain = types.StringValue(endpoint.Chain)
	config.Network = types.StringValue(endpoint.Network)
	config.Status = types.StringValue(endpoint.Status)
	config.Multichain = types.BoolValue(endpoint.Multichain)
	config.HTTPURL = stringOrNull(endpoint.HTTPURL)
	config.WSSURL = stringOrNull(endpoint.WSSURL)
	config.HTTPURLWithToken = stringOrNull(endpoint.HTTPURLWithToken)
	config.WSSURLWithToken = stringOrNull(endpoint.WSSURLWithToken)
	config.IPCustomHeader = stringOrNull(endpoint.Security.IPCustomHeader)

	config.Tags = make([]types.String, 0, len(endpoint.Tags))
	for _, tag := range endpoint.Tags {
		config.Tags = append(config.Tags, types.StringValue(tag.Label))
	}
	config.Tokens = make([]tokenModel, 0, len(endpoint.Tokens))
	for _, token := range endpoint.Tokens {
		config.Tokens = append(config.Tokens, tokenModel{
			ID:    types.StringValue(token.ID),
			Token: types.StringValue(token.Value),
		})
	}

	options, diags := securityOptionsObject(endpoint.Security)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	config.SecurityOptions = options
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
