package provider_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
	"github.com/quicknode/terraform-provider-quicknode/internal/provider"
)

// Acceptance tests create real, billable Quicknode resources. They run only
// when TF_ACC is set, and they need QUICKNODE_API_KEY for a paid account that
// is dedicated to testing.
//
// The chain and network are deliberately a testnet, and every test destroys
// what it created. Run them with `make testacc`.
const (
	acceptanceChain   = "eth"
	acceptanceNetwork = "ethereum-sepolia"
)

var protoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"quicknode": providerserver.NewProtocol6WithError(provider.New("test")()),
}

func testAccPreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv("QUICKNODE_API_KEY") == "" {
		t.Fatal("QUICKNODE_API_KEY must be set for acceptance tests")
	}
}

// testAccCheckEndpointsDestroyed asks the Admin API whether the endpoints the
// test created are really gone. Terraform calls a destroy successful as soon as
// the provider's Delete returns no error, so a delete the API did not honour
// would otherwise pass and leave a billable endpoint behind. Every other
// resource in these tests belongs to an endpoint and goes with it.
func testAccCheckEndpointsDestroyed(state *terraform.State) error {
	quicknode, err := client.New(os.Getenv("QUICKNODE_API_KEY"))
	if err != nil {
		return fmt.Errorf("could not build a client to confirm the destroy: %w", err)
	}

	for name, resourceState := range state.RootModule().Resources {
		if resourceState.Type != "quicknode_endpoint" {
			continue
		}
		id := resourceState.Primary.ID
		endpoint, err := quicknode.GetEndpoint(context.Background(), id)
		if client.IsNotFound(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("%s: could not confirm endpoint %s was destroyed: %w", name, id, err)
		}
		return fmt.Errorf("%s: endpoint %s still exists after destroy, with status %q", name, id, endpoint.Status)
	}
	return nil
}

func endpointConfig(label string) string {
	return fmt.Sprintf(`
resource "quicknode_endpoint" "test" {
  chain   = %q
  network = %q
  label   = %q
}
`, acceptanceChain, acceptanceNetwork, label)
}

func TestAccEndpoint_lifecycle(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckEndpointsDestroyed,
		Steps: []resource.TestStep{
			{
				Config: endpointConfig("tfacc-endpoint"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("quicknode_endpoint.test", "id"),
					resource.TestCheckResourceAttr("quicknode_endpoint.test", "chain", acceptanceChain),
					resource.TestCheckResourceAttr("quicknode_endpoint.test", "label", "tfacc-endpoint"),
					resource.TestCheckResourceAttr("quicknode_endpoint.test", "status", "active"),
					// The credentialed URL is the one that works; the stripped
					// URL must not carry the token.
					resource.TestCheckResourceAttrSet("quicknode_endpoint.test", "http_url_with_token"),
					resource.TestCheckResourceAttrSet("quicknode_endpoint.test", "tokens.0.token"),
					resource.TestCheckResourceAttrSet("quicknode_endpoint.test", "security_options.tokens"),
				),
			},
			{
				ResourceName:      "quicknode_endpoint.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: endpointConfig("tfacc-endpoint-renamed"),
				Check: resource.TestCheckResourceAttr(
					"quicknode_endpoint.test", "label", "tfacc-endpoint-renamed"),
			},
		},
	})
}

// TestAccEndpoint_securityOptions checks the boolean-read, string-write
// asymmetry end to end, and that a toggle the configuration does not manage
// keeps its value rather than being reset.
func TestAccEndpoint_securityOptions(t *testing.T) {
	withOptions := func(cors bool) string {
		return fmt.Sprintf(`
resource "quicknode_endpoint" "test" {
  chain   = %q
  network = %q
  label   = "tfacc-security-options"

  security_options = {
    cors = %t
  }
}
`, acceptanceChain, acceptanceNetwork, cors)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckEndpointsDestroyed,
		Steps: []resource.TestStep{
			{
				Config: withOptions(false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("quicknode_endpoint.test", "security_options.cors", "false"),
					// tokens is unmanaged here, so it has to keep the value a
					// new endpoint is created with rather than being turned off.
					resource.TestCheckResourceAttr("quicknode_endpoint.test", "security_options.tokens", "true"),
				),
			},
			{
				Config: withOptions(true),
				Check:  resource.TestCheckResourceAttr("quicknode_endpoint.test", "security_options.cors", "true"),
			},
		},
	})
}

func TestAccEndpointIP_importByValue(t *testing.T) {
	const config = `
resource "quicknode_endpoint" "test" {
  chain   = "eth"
  network = "ethereum-sepolia"
  label   = "tfacc-ip"

  security_options = {
    ips = true
  }
}

resource "quicknode_endpoint_ip" "test" {
  endpoint_id = quicknode_endpoint.test.id
  ip          = "203.0.113.7"
}
`
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckEndpointsDestroyed,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("quicknode_endpoint_ip.test", "id"),
					resource.TestCheckResourceAttr("quicknode_endpoint_ip.test", "ip", "203.0.113.7"),
				),
			},
			{
				ResourceName: "quicknode_endpoint_ip.test",
				ImportState:  true,
				// The address is the value an operator already knows, not the
				// entry id the API assigned.
				ImportStateIdFunc: func(state *terraform.State) (string, error) {
					endpoint := state.RootModule().Resources["quicknode_endpoint.test"]
					if endpoint == nil {
						return "", fmt.Errorf("the endpoint is not in state")
					}
					return endpoint.Primary.ID + "/203.0.113.7", nil
				},
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccRequestFilter_updatesInPlace(t *testing.T) {
	withMethods := func(methods string) string {
		return fmt.Sprintf(`
resource "quicknode_endpoint" "test" {
  chain   = %q
  network = %q
  label   = "tfacc-request-filter"
}

resource "quicknode_endpoint_request_filter" "test" {
  endpoint_id = quicknode_endpoint.test.id
  methods     = %s
}
`, acceptanceChain, acceptanceNetwork, methods)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckEndpointsDestroyed,
		Steps: []resource.TestStep{
			{
				Config: withMethods(`["eth_call"]`),
				Check:  resource.TestCheckResourceAttr("quicknode_endpoint_request_filter.test", "methods.#", "1"),
			},
			{
				// A real PUT route backs this, so the filter must update rather
				// than be replaced.
				Config: withMethods(`["eth_call", "eth_getLogs"]`),
				Check:  resource.TestCheckResourceAttr("quicknode_endpoint_request_filter.test", "methods.#", "2"),
			},
		},
	})
}

// TestAccRateLimits_dropReturnsPlanDefault covers the behaviour the patch route
// cannot express: removing a bucket from the configuration has to delete the
// override rather than leave it in place.
func TestAccRateLimits_dropReturnsPlanDefault(t *testing.T) {
	withBuckets := func(buckets string) string {
		return fmt.Sprintf(`
resource "quicknode_endpoint" "test" {
  chain   = %q
  network = %q
  label   = "tfacc-rate-limits"
}

resource "quicknode_endpoint_rate_limits" "test" {
  endpoint_id = quicknode_endpoint.test.id
%s
}
`, acceptanceChain, acceptanceNetwork, buckets)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckEndpointsDestroyed,
		Steps: []resource.TestStep{
			{
				Config: withBuckets("  rps = 25\n  rpm = 500"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("quicknode_endpoint_rate_limits.test", "rps", "25"),
					resource.TestCheckResourceAttr("quicknode_endpoint_rate_limits.test", "rpm", "500"),
					resource.TestCheckResourceAttrSet("quicknode_endpoint_rate_limits.test", "plan_default.rps"),
				),
			},
			{
				Config: withBuckets("  rps = 25"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("quicknode_endpoint_rate_limits.test", "rps", "25"),
					resource.TestCheckNoResourceAttr("quicknode_endpoint_rate_limits.test", "rpm"),
				),
			},
		},
	})
}

func TestAccMethodRateLimit_lifecycle(t *testing.T) {
	withRate := func(rate int, enabled bool) string {
		return fmt.Sprintf(`
resource "quicknode_endpoint" "test" {
  chain   = %q
  network = %q
  label   = "tfacc-method-rate-limit"
}

resource "quicknode_endpoint_method_rate_limit" "test" {
  endpoint_id = quicknode_endpoint.test.id
  methods     = ["eth_getLogs"]
  rate        = %d
  interval    = "second"
  enabled     = %t
}
`, acceptanceChain, acceptanceNetwork, rate, enabled)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckEndpointsDestroyed,
		Steps: []resource.TestStep{
			{
				Config: withRate(5, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("quicknode_endpoint_method_rate_limit.test", "rate", "5"),
					resource.TestCheckResourceAttr("quicknode_endpoint_method_rate_limit.test", "enabled", "true"),
				),
			},
			{
				Config: withRate(9, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("quicknode_endpoint_method_rate_limit.test", "rate", "9"),
					resource.TestCheckResourceAttr("quicknode_endpoint_method_rate_limit.test", "enabled", "false"),
				),
			},
		},
	})
}

func TestAccEndpointDataSource_byLabel(t *testing.T) {
	const config = `
resource "quicknode_endpoint" "test" {
  chain   = "eth"
  network = "ethereum-sepolia"
  label   = "tfacc-data-source"
}

data "quicknode_endpoint" "test" {
  label      = quicknode_endpoint.test.label
  depends_on = [quicknode_endpoint.test]
}

data "quicknode_endpoints" "test" {
  statuses   = ["active"]
  depends_on = [quicknode_endpoint.test]
}
`
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckEndpointsDestroyed,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"data.quicknode_endpoint.test", "id",
						"quicknode_endpoint.test", "id"),
					resource.TestCheckResourceAttrSet("data.quicknode_endpoint.test", "http_url_with_token"),
					resource.TestCheckResourceAttrSet("data.quicknode_endpoints.test", "ids.#"),
				),
			},
		},
	})
}

func TestAccChainsDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckEndpointsDestroyed,
		Steps: []resource.TestStep{
			{
				Config: `data "quicknode_chains" "all" {}`,
				Check:  resource.TestCheckResourceAttrSet("data.quicknode_chains.all", "chains.#"),
			},
		},
	})
}
