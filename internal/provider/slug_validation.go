package provider

import (
	"fmt"
	"sort"
	"strings"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

// slugProblem is a rejected chain or network, named by the attribute it belongs
// to so the caller can attach it to the right path.
type slugProblem struct {
	Attribute string
	Summary   string
	Detail    string
}

// validateChainNetwork matches slugs exactly, casing included.
// The Admin API echoes back its own casing, and chain and network force
// replacement, so accepting "ETH" for "eth" would let Read overwrite the
// configured value on the next refresh and leave every subsequent plan
// proposing a destroy and create that never converges.
func validateChainNetwork(chains []client.Chain, wantChain, wantNetwork string) *slugProblem {
	if len(chains) == 0 {
		return nil
	}

	for _, chain := range chains {
		if chain.Slug != wantChain {
			continue
		}
		networkSlugs := make([]string, 0, len(chain.Networks))
		for _, network := range chain.Networks {
			if network.Slug == wantNetwork {
				return nil
			}
			networkSlugs = append(networkSlugs, network.Slug)
		}
		for _, network := range chain.Networks {
			if strings.EqualFold(network.Slug, wantNetwork) {
				return &slugProblem{
					Attribute: "network",
					Summary:   "Network slug differs in case",
					Detail:    fmt.Sprintf("Use %q, not %q. Quicknode reports its own casing back, and network forces replacement, so a mismatch would make every plan propose a replacement.", network.Slug, wantNetwork),
				}
			}
		}
		sort.Strings(networkSlugs)
		return &slugProblem{
			Attribute: "network",
			Summary:   "Unknown network for this chain",
			Detail:    fmt.Sprintf("Chain %q has no network %q. Its networks are: %s.", wantChain, wantNetwork, strings.Join(networkSlugs, ", ")),
		}
	}

	chainSlugs := make([]string, 0, len(chains))
	for _, chain := range chains {
		if strings.EqualFold(chain.Slug, wantChain) {
			return &slugProblem{
				Attribute: "chain",
				Summary:   "Chain slug differs in case",
				Detail:    fmt.Sprintf("Use %q, not %q. Quicknode reports its own casing back, and chain forces replacement, so a mismatch would make every plan propose a replacement.", chain.Slug, wantChain),
			}
		}
		chainSlugs = append(chainSlugs, chain.Slug)
	}

	sort.Strings(chainSlugs)
	return &slugProblem{
		Attribute: "chain",
		Summary:   "Unknown chain",
		Detail:    fmt.Sprintf("Quicknode has no chain %q. Chain slugs are abbreviations, so Ethereum is \"eth\" and Polygon is \"matic\". Known chains: %s.", wantChain, strings.Join(chainSlugs, ", ")),
	}
}
