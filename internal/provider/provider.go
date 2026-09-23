package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

const apiKeyEnvVar = "QUICKNODE_API_KEY"

// legacyAPIKeyEnvVars are the variables the community providers read. The
// official provider accepts them so a migrating configuration keeps working.
var legacyAPIKeyEnvVars = []string{"QUICKNODE_APIKEY", "QUICKNODE_API_TOKEN"}

var _ provider.Provider = (*quicknodeProvider)(nil)

type quicknodeProvider struct {
	version string
}

type providerModel struct {
	APIKey            types.String `tfsdk:"api_key"`
	BaseURL           types.String `tfsdk:"base_url"`
	RequestsPerSecond types.Int64  `tfsdk:"requests_per_second"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &quicknodeProvider{version: version}
	}
}

func (p *quicknodeProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "quicknode"
	resp.Version = p.version
}

func (p *quicknodeProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage Quicknode RPC infrastructure as code.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Quicknode API key. Defaults to the `" + apiKeyEnvVar + "` environment variable. Prefer the environment variable so the key stays out of configuration and state.",
			},
			"base_url": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Quicknode API base URL. Defaults to `" + client.DefaultBaseURL + "`.",
			},
			"requests_per_second": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Throttle applied to Quicknode API calls. A large workspace bursts many calls during one apply, so the provider paces itself.",
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
			},
		},
	}
}

func (p *quicknodeProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.APIKey.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_key"),
			"API key is not known at plan time",
			"The api_key value comes from another resource that has not been applied yet. Set it from a variable or from the "+apiKeyEnvVar+" environment variable instead.",
		)
		return
	}

	apiKey := os.Getenv(apiKeyEnvVar)
	for _, legacy := range legacyAPIKeyEnvVars {
		if apiKey != "" {
			break
		}
		apiKey = os.Getenv(legacy)
	}
	if !config.APIKey.IsNull() && config.APIKey.ValueString() != "" {
		apiKey = config.APIKey.ValueString()
	}
	if apiKey == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_key"),
			"Missing Quicknode API key",
			"Set the "+apiKeyEnvVar+" environment variable, or set api_key on the provider block. API access requires a paid Quicknode plan.",
		)
		return
	}

	quicknode, err := client.New(apiKey,
		client.WithBaseURL(config.BaseURL.ValueString()),
		client.WithRequestsPerSecond(int(config.RequestsPerSecond.ValueInt64())),
	)
	if err != nil {
		resp.Diagnostics.AddError("Could not build the Quicknode API client", err.Error())
		return
	}

	chains, err := quicknode.ListChains(ctx)
	if err != nil {
		if client.IsUnauthorized(err) {
			resp.Diagnostics.AddError(
				"Quicknode rejected the API key",
				"Check that the key is valid and that the account is on a paid plan, which API access requires. "+err.Error(),
			)
			return
		}
		resp.Diagnostics.AddError("Could not reach the Quicknode API", err.Error())
		return
	}

	data := providerData{Client: quicknode, Chains: chains}
	resp.DataSourceData = data
	resp.ResourceData = data
}

func (p *quicknodeProvider) Resources(_ context.Context) []func() resource.Resource {
	resources := []func() resource.Resource{
		NewEndpointResource,
		NewEndpointTokenResource,
		NewJWTResource,
		NewRequestFilterResource,
		NewRateLimitsResource,
		NewMethodRateLimitResource,
	}
	return append(resources, securityEntryResources()...)
}

func (p *quicknodeProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewChainsDataSource,
		NewEndpointDataSource,
		NewEndpointsDataSource,
	}
}
