package provider

import (
	"strings"
	"testing"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

func testChains() []client.Chain {
	return []client.Chain{
		{Slug: "eth", Networks: []client.Network{
			{Slug: "mainnet"}, {Slug: "ethereum-sepolia"}, {Slug: "ethereum-hoodi"},
		}},
		{Slug: "base", Networks: []client.Network{
			{Slug: "base-mainnet"}, {Slug: "base-sepolia"},
		}},
	}
}

func TestValidateChainNetwork(t *testing.T) {
	cases := []struct {
		name        string
		chain       string
		network     string
		wantAttr    string
		wantDetails string
	}{
		{name: "exact match", chain: "eth", network: "mainnet"},
		{name: "exact match on a qualified network", chain: "base", network: "base-sepolia"},
		{
			name: "chain case mismatch is rejected", chain: "ETH", network: "mainnet",
			wantAttr: "chain", wantDetails: `Use "eth" rather than "ETH"`,
		},
		{
			name: "network case mismatch is rejected", chain: "base", network: "Base-Sepolia",
			wantAttr: "network", wantDetails: `Use "base-sepolia" rather than "Base-Sepolia"`,
		},
		{
			name: "chain name instead of slug", chain: "ethereum", network: "mainnet",
			wantAttr: "chain", wantDetails: "has no chain",
		},
		{
			name: "network from another chain", chain: "eth", network: "base-sepolia",
			wantAttr: "network", wantDetails: "has no network",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			problem := validateChainNetwork(testChains(), testCase.chain, testCase.network)

			if testCase.wantAttr == "" {
				if problem != nil {
					t.Fatalf("expected no problem, got %s: %s", problem.Summary, problem.Detail)
				}
				return
			}
			if problem == nil {
				t.Fatal("expected a problem, got none")
			}
			if problem.Attribute != testCase.wantAttr {
				t.Errorf("attribute = %q, want %q", problem.Attribute, testCase.wantAttr)
			}
			if !strings.Contains(problem.Detail, testCase.wantDetails) {
				t.Errorf("detail = %q, want it to contain %q", problem.Detail, testCase.wantDetails)
			}
		})
	}
}

func TestValidateChainNetworkSkipsWhenChainsUnavailable(t *testing.T) {
	if problem := validateChainNetwork(nil, "nonsense", "nonsense"); problem != nil {
		t.Errorf("expected validation to be skipped without a chain list, got %s", problem.Summary)
	}
}
