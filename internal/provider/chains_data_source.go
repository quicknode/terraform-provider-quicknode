package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

var _ datasource.DataSource = (*chainsDataSource)(nil)
var _ datasource.DataSourceWithConfigure = (*chainsDataSource)(nil)

type chainsDataSource struct {
	client *client.Client
}

type chainsDataSourceModel struct {
	Chains []chainModel `tfsdk:"chains"`
}

type chainModel struct {
	Slug          types.String   `tfsdk:"slug"`
	IsSelectChain types.Bool     `tfsdk:"is_select_chain"`
	Networks      []networkModel `tfsdk:"networks"`
}

type networkModel struct {
	Slug    types.String `tfsdk:"slug"`
	Name    types.String `tfsdk:"name"`
	ChainID types.Int64  `tfsdk:"chain_id"`
}

func NewChainsDataSource() datasource.DataSource {
	return &chainsDataSource{}
}

func (d *chainsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_chains"
}

func (d *chainsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Every chain and network Quicknode supports. Use it to validate slugs at plan time and to fan a configuration out across networks.\n\n" +
			"Chain slugs are abbreviations that often differ from the chain's name: Ethereum is `eth`, Avalanche is `avax`, Arbitrum is `arb`, Polygon is `matic`.",
		Attributes: map[string]schema.Attribute{
			"chains": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"slug": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Chain slug, as `quicknode_endpoint.chain` expects it.",
						},
						"is_select_chain": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether the chain is only available on Quicknode's Select plans.",
						},
						"networks": schema.ListNestedAttribute{
							Computed: true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"slug": schema.StringAttribute{
										Computed:            true,
										MarkdownDescription: "Network slug, as `quicknode_endpoint.network` expects it. Most are chain-qualified (`base-sepolia`), but some are not (`mainnet` for Ethereum, `bsc`, `optimism`).",
									},
									"name": schema.StringAttribute{
										Computed:            true,
										MarkdownDescription: "Human-readable network name.",
									},
									"chain_id": schema.Int64Attribute{
										Computed:            true,
										MarkdownDescription: "EVM chain id, or null on non-EVM networks.",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (d *chainsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("The chains data source expected providerData, got %T.", req.ProviderData))
		return
	}
	d.client = data.Client
}

func (d *chainsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	chains, err := d.client.ListChains(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Could not list Quicknode chains", err.Error())
		return
	}

	state := chainsDataSourceModel{Chains: make([]chainModel, 0, len(chains))}
	for _, chain := range chains {
		entry := chainModel{
			Slug:          types.StringValue(chain.Slug),
			IsSelectChain: types.BoolValue(chain.IsSelectChain),
			Networks:      make([]networkModel, 0, len(chain.Networks)),
		}
		for _, network := range chain.Networks {
			entry.Networks = append(entry.Networks, networkModel{
				Slug:    types.StringValue(network.Slug),
				Name:    types.StringValue(network.Name),
				ChainID: chainIDValue(network.ChainID),
			})
		}
		state.Chains = append(state.Chains, entry)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func chainIDValue(chainID *int64) types.Int64 {
	if chainID == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*chainID)
}
