package provider_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

// The streams in these tests stay paused, so nothing is ever delivered to the
// example.com receiver. Quicknode does not contact a webhook when a stream is
// created, only when it delivers.
const streamAcceptanceReceiver = "https://example.com/quicknode-tfacc"

const streamAcceptanceFilter = `function main(stream) {
  return { number: stream.data[0].number };
}
`

func testAccCheckStreamsDestroyed(state *terraform.State) error {
	quicknode, err := client.New(os.Getenv("QUICKNODE_API_KEY"))
	if err != nil {
		return fmt.Errorf("could not build a client to confirm the destroy: %w", err)
	}

	for name, resourceState := range state.RootModule().Resources {
		if resourceState.Type != "quicknode_stream" {
			continue
		}
		id := resourceState.Primary.ID
		stream, err := quicknode.GetStream(context.Background(), id)
		if client.IsNotFound(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("%s: could not confirm stream %s was destroyed: %w", name, id, err)
		}
		return fmt.Errorf("%s: stream %s still exists after destroy, with status %q", name, id, stream.Status)
	}
	return nil
}

// streamConfig renders a Sepolia stream. settings is added to the resource
// body and destination replaces the webhook destination when set.
func streamConfig(name, status, settings, destination string) string {
	if destination == "" {
		destination = fmt.Sprintf(`{
    webhook = {
      url     = %q
      headers = { "X-Tfacc" = "true" }
    }
  }`, streamAcceptanceReceiver)
	}
	return fmt.Sprintf(`
resource "quicknode_stream" "test" {
  name            = %q
  network         = %q
  dataset         = "block"
  region          = "usa_east"
  status          = %q
  filter_function = %q
%s
  destination = %s
}
`, name, acceptanceNetwork, status, streamAcceptanceFilter, settings, destination)
}

func TestAccStream_lifecycle(t *testing.T) {
	extraDestination := fmt.Sprintf(`
  extra_destinations = [{
    webhook = {
      url         = %q
      compression = "gzip"
    }
  }]
`, streamAcceptanceReceiver+"/extra")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckStreamsDestroyed,
		Steps: []resource.TestStep{
			{
				Config: streamConfig("tfacc-stream", "paused", "", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("quicknode_stream.test", "id"),
					resource.TestCheckResourceAttr("quicknode_stream.test", "state", "paused"),
					resource.TestCheckResourceAttr("quicknode_stream.test", "filter_function", streamAcceptanceFilter),
					resource.TestCheckResourceAttrSet("quicknode_stream.test", "start_range"),
					resource.TestCheckNoResourceAttr("quicknode_stream.test", "end_range"),
					resource.TestMatchResourceAttr("quicknode_stream.test", "destination.webhook.security_token", regexp.MustCompile(`.+`)),
					resource.TestCheckResourceAttr("quicknode_stream.test", "destination.webhook.post_timeout_sec", "30"),
					resource.TestCheckResourceAttr("quicknode_stream.test", "destination.webhook.max_retry", "3"),
					resource.TestCheckNoResourceAttr("quicknode_stream.test", "extra_destinations"),
				),
			},
			{
				ResourceName:      "quicknode_stream.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: streamConfig("tfacc-stream-renamed", "paused", extraDestination, ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("quicknode_stream.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("quicknode_stream.test", "name", "tfacc-stream-renamed"),
					resource.TestCheckResourceAttr("quicknode_stream.test", "extra_destinations.#", "1"),
					resource.TestCheckResourceAttr("quicknode_stream.test", "extra_destinations.0.webhook.compression", "gzip"),
					resource.TestCheckNoResourceAttr("quicknode_stream.test", "extra_destinations.0.webhook.security_token"),
				),
			},
			{
				Config: streamConfig("tfacc-stream-renamed", "paused", "", ""),
				Check:  resource.TestCheckNoResourceAttr("quicknode_stream.test", "extra_destinations"),
			},
			{
				Config: streamConfig("tfacc-stream-renamed", "paused", "  extra_destinations = []\n", ""),
				Check:  resource.TestCheckResourceAttr("quicknode_stream.test", "extra_destinations.#", "0"),
			},
			{
				Config: streamConfig("tfacc-stream-renamed", "paused", "", ""),
				Check:  resource.TestCheckNoResourceAttr("quicknode_stream.test", "extra_destinations"),
			},
		},
	})
}

func TestAccStreamDataSource_byID(t *testing.T) {
	config := streamConfig("tfacc-stream-data-source", "paused", "", "") + `
data "quicknode_stream" "test" {
  id = quicknode_stream.test.id
}
`
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckStreamsDestroyed,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.quicknode_stream.test", "name", "quicknode_stream.test", "name"),
					resource.TestCheckResourceAttrPair("data.quicknode_stream.test", "status", "quicknode_stream.test", "state"),
					resource.TestCheckResourceAttrPair("data.quicknode_stream.test", "start_range", "quicknode_stream.test", "start_range"),
					resource.TestCheckResourceAttrPair("data.quicknode_stream.test", "filter_function", "quicknode_stream.test", "filter_function"),
					resource.TestCheckResourceAttrPair(
						"data.quicknode_stream.test", "destination.webhook.security_token",
						"quicknode_stream.test", "destination.webhook.security_token"),
					resource.TestCheckResourceAttr("data.quicknode_stream.test", "destination.webhook.headers.X-Tfacc", "true"),
					resource.TestCheckNoResourceAttr("data.quicknode_stream.test", "destination.s3"),
					resource.TestCheckResourceAttrSet("data.quicknode_stream.test", "sequence"),
				),
			},
		},
	})
}

// TestAccStream_planRules checks the rules the provider enforces at plan time.
// None of the plan-only steps apply, so the S3 credentials are never used.
func TestAccStream_planRules(t *testing.T) {
	const s3Destination = `{
    s3 = {
      bucket     = "tfacc-never-created"
      region     = "us-east-1"
      access_key = "AKIAEXAMPLE"
      secret_key = "not-a-real-secret"
    }
  }`
	const conditionalDestination = `{
    webhook = var.use_postgres ? null : { url = "https://example.com/quicknode-tfacc" }
    postgres = var.use_postgres ? {
      host       = "db.example.com"
      database   = "tfacc"
      table_name = "blocks"
      username   = "tfacc"
      password   = "not-a-real-password"
    } : null
  }`
	const useThePostgresDestination = `
variable "use_postgres" {
  type    = bool
  default = true
}
`

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckStreamsDestroyed,
		Steps: []resource.TestStep{
			{
				Config:      streamConfig("tfacc-stream-rules", "paused", "  elastic_batch_enabled = true\n  end_range = 100", ""),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Elastic batching needs an open-ended stream`),
			},
			{
				Config: streamConfig("tfacc-stream-rules", "paused", "", ""),
			},
			{
				Config:      streamConfig("tfacc-stream-rules", "active", "  resume_from = \"latest\"\n  start_range = 1", ""),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`resume_from = "latest" conflicts with start_range`),
			},
			{
				Config:             streamConfig("tfacc-stream-rules", "paused", "", s3Destination),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPreRefresh: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("quicknode_stream.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
			},
			{
				Config:             useThePostgresDestination + streamConfig("tfacc-stream-rules", "paused", "", conditionalDestination),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPreRefresh: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("quicknode_stream.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
			},
		},
	})
}
