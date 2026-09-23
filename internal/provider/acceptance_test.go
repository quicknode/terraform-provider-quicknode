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
// keeps its value.
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
					// new endpoint is created with.
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
// override.
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

// jwtAcceptancePublicKey is a throwaway RSA public key, generated for these
// tests and used nowhere else. The matching private key was discarded.
const jwtAcceptancePublicKey = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAvxK0qszzlluhCeocaEdM
RRoXn+2tmIyi/ToD8YmXQd9KhqHJLs2zT6WAEctEr3AetcJmWpwyXEaMzSn6aoIn
SBmic1PeGCyecISo2wt/RkbdoWYJe48T4BOAwH/ljOztYjldfiVKINNTR8975K1Z
uo6QOo//grLrZOzqU7BTG8r7MqyX8eAh/W3MEthhpFEIlLCOyxL0HMwOsfbTYG9/
EvmvmBTqnXfU653cFnAWjtHBVO3YOcp/CmaU+4pqF+Pu+prGajLIekzZnjIx0W6j
mY6xngVlYGHdIj1qCcvETrSRaz6JemJpBseFNaOUe7AYDl2Ne5UPnkdgDTZUOuj5
/QIDAQAB
-----END PUBLIC KEY-----
`

// endpointIDForImport addresses a child resource by the endpoint it belongs to,
// which is how every one of them is imported.
func endpointIDForImport(suffix func(*terraform.State) (string, error)) func(*terraform.State) (string, error) {
	return func(state *terraform.State) (string, error) {
		endpoint := state.RootModule().Resources["quicknode_endpoint.test"]
		if endpoint == nil {
			return "", fmt.Errorf("the endpoint is not in state")
		}
		tail, err := suffix(state)
		if err != nil {
			return "", err
		}
		return endpoint.Primary.ID + "/" + tail, nil
	}
}

// TestAccEndpoint_labelSurvivesRemoval covers the one attribute Quicknode has
// no route to clear. Dropping it from the configuration has to leave the
// endpoint's label alone and settle into an empty plan.
func TestAccEndpoint_labelSurvivesRemoval(t *testing.T) {
	unlabelled := fmt.Sprintf(`
resource "quicknode_endpoint" "test" {
  chain   = %q
  network = %q
}
`, acceptanceChain, acceptanceNetwork)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckEndpointsDestroyed,
		Steps: []resource.TestStep{
			{
				Config: endpointConfig("tfacc-label"),
				Check:  resource.TestCheckResourceAttr("quicknode_endpoint.test", "label", "tfacc-label"),
			},
			{
				Config: unlabelled,
				Check:  resource.TestCheckResourceAttr("quicknode_endpoint.test", "label", "tfacc-label"),
			},
		},
	})
}

// TestAccEndpoint_tagsStatusAndHeader exercises the attributes that reconcile
// against the API on update: tags are added and removed one at a time, the
// status is paused and resumed, and clearing the custom IP header is a delete
// call of its own.
func TestAccEndpoint_tagsStatusAndHeader(t *testing.T) {
	withAttributes := func(body string) string {
		return fmt.Sprintf(`
resource "quicknode_endpoint" "test" {
  chain   = %q
  network = %q
  label   = "tfacc-attributes"
%s
}
`, acceptanceChain, acceptanceNetwork, body)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckEndpointsDestroyed,
		Steps: []resource.TestStep{
			{
				Config: withAttributes(`  tags             = ["tfacc-one", "tfacc-two"]
  status           = "paused"
  ip_custom_header = "X-Real-IP"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("quicknode_endpoint.test", "tags.#", "2"),
					resource.TestCheckTypeSetElemAttr("quicknode_endpoint.test", "tags.*", "tfacc-one"),
					resource.TestCheckTypeSetElemAttr("quicknode_endpoint.test", "tags.*", "tfacc-two"),
					resource.TestCheckResourceAttr("quicknode_endpoint.test", "status", "paused"),
					resource.TestCheckResourceAttr("quicknode_endpoint.test", "ip_custom_header", "X-Real-IP"),
				),
			},
			{
				// One tag stays, one goes and one arrives, so the reconcile has
				// to issue both an add and a remove in the same apply.
				Config: withAttributes(`  tags             = ["tfacc-two", "tfacc-three"]
  status           = "active"
  ip_custom_header = "X-Forwarded-For"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("quicknode_endpoint.test", "tags.#", "2"),
					resource.TestCheckTypeSetElemAttr("quicknode_endpoint.test", "tags.*", "tfacc-three"),
					resource.TestCheckResourceAttr("quicknode_endpoint.test", "status", "active"),
					resource.TestCheckResourceAttr("quicknode_endpoint.test", "ip_custom_header", "X-Forwarded-For"),
				),
			},
			{
				Config: withAttributes(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("quicknode_endpoint.test", "tags.#", "0"),
					resource.TestCheckNoResourceAttr("quicknode_endpoint.test", "ip_custom_header"),
				),
			},
		},
	})
}

func TestAccEndpointToken_lifecycle(t *testing.T) {
	config := fmt.Sprintf(`
resource "quicknode_endpoint" "test" {
  chain   = %q
  network = %q
  label   = "tfacc-token"
}

resource "quicknode_endpoint_token" "test" {
  endpoint_id = quicknode_endpoint.test.id
}
`, acceptanceChain, acceptanceNetwork)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckEndpointsDestroyed,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("quicknode_endpoint_token.test", "id"),
					resource.TestCheckResourceAttrSet("quicknode_endpoint_token.test", "token"),
				),
			},
			{
				// The endpoint is created carrying one token and is not read
				// again during the apply that adds the second, so the count on
				// the endpoint only settles on the next refresh.
				Config: config,
				Check:  resource.TestCheckResourceAttr("quicknode_endpoint.test", "tokens.#", "2"),
			},
			{
				ResourceName: "quicknode_endpoint_token.test",
				ImportState:  true,
				ImportStateIdFunc: endpointIDForImport(func(state *terraform.State) (string, error) {
					return state.RootModule().Resources["quicknode_endpoint_token.test"].Primary.ID, nil
				}),
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccEndpointJWT_importByName(t *testing.T) {
	config := fmt.Sprintf(`
resource "quicknode_endpoint" "test" {
  chain   = %q
  network = %q
  label   = "tfacc-jwt"

  security_options = {
    jwts = true
  }
}

resource "quicknode_endpoint_jwt" "test" {
  endpoint_id = quicknode_endpoint.test.id
  name        = "tfacc-signer"
  kid         = "tfacc-kid"
  public_key  = <<-EOT
%s
EOT
}
`, acceptanceChain, acceptanceNetwork, jwtAcceptancePublicKey)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckEndpointsDestroyed,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("quicknode_endpoint_jwt.test", "id"),
					resource.TestCheckResourceAttr("quicknode_endpoint_jwt.test", "kid", "tfacc-kid"),
					resource.TestCheckResourceAttr("quicknode_endpoint_jwt.test", "name", "tfacc-signer"),
				),
			},
			{
				ResourceName: "quicknode_endpoint_jwt.test",
				ImportState:  true,
				ImportStateIdFunc: endpointIDForImport(func(*terraform.State) (string, error) {
					return "tfacc-signer", nil
				}),
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccSecurityEntries_importByValue covers the two allowlists the IP test
// does not, on one endpoint, since each kind shares an implementation and
// differs only in its routes.
func TestAccSecurityEntries_importByValue(t *testing.T) {
	config := fmt.Sprintf(`
resource "quicknode_endpoint" "test" {
  chain   = %q
  network = %q
  label   = "tfacc-entries"

  security_options = {
    domain_masks = true
    referrers    = true
  }
}

resource "quicknode_endpoint_domain_mask" "test" {
  endpoint_id = quicknode_endpoint.test.id
  domain      = "tfacc.example.com"
}

resource "quicknode_endpoint_referrer" "test" {
  endpoint_id = quicknode_endpoint.test.id
  referrer    = "https://tfacc.example.com"
}
`, acceptanceChain, acceptanceNetwork)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckEndpointsDestroyed,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("quicknode_endpoint_domain_mask.test", "domain", "tfacc.example.com"),
					resource.TestCheckResourceAttr("quicknode_endpoint_referrer.test", "referrer", "https://tfacc.example.com"),
					resource.TestCheckResourceAttr("quicknode_endpoint.test", "security_options.domain_masks", "true"),
					resource.TestCheckResourceAttr("quicknode_endpoint.test", "security_options.referrers", "true"),
				),
			},
			{
				ResourceName: "quicknode_endpoint_domain_mask.test",
				ImportState:  true,
				ImportStateIdFunc: endpointIDForImport(func(*terraform.State) (string, error) {
					return "tfacc.example.com", nil
				}),
				ImportStateVerify: true,
			},
			{
				ResourceName: "quicknode_endpoint_referrer.test",
				ImportState:  true,
				ImportStateIdFunc: endpointIDForImport(func(*terraform.State) (string, error) {
					return "https://tfacc.example.com", nil
				}),
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccSecurityEntry_survivesDisabledToggle covers the read hazard behind the
// order the documentation recommends. The security route omits a list entirely
// while its toggle is disabled, so an entry added before the toggle is turned
// on has to stay in state and leave the plan empty.
func TestAccSecurityEntry_survivesDisabledToggle(t *testing.T) {
	config := fmt.Sprintf(`
resource "quicknode_endpoint" "test" {
  chain   = %q
  network = %q
  label   = "tfacc-disabled-toggle"

  security_options = {
    ips = false
  }
}

resource "quicknode_endpoint_ip" "test" {
  endpoint_id = quicknode_endpoint.test.id
  ip          = "203.0.113.9"
}
`, acceptanceChain, acceptanceNetwork)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckEndpointsDestroyed,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("quicknode_endpoint_ip.test", "ip", "203.0.113.9"),
					resource.TestCheckResourceAttr("quicknode_endpoint.test", "security_options.ips", "false"),
				),
			},
		},
	})
}
