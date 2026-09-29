package provider_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/quicknode/terraform-provider-quicknode/internal/client"
)

// Key-value keys are account wide, so every test uses a random one.
func kvAcceptanceKey() string {
	return acctest.RandomWithPrefix("tfacc")
}

func testAccClient(t *testing.T) *client.Client {
	t.Helper()
	quicknode, err := client.New(os.Getenv("QUICKNODE_API_KEY"))
	if err != nil {
		t.Fatalf("could not build a client: %v", err)
	}
	return quicknode
}

func testAccCheckKVDestroyed(state *terraform.State) error {
	quicknode, err := client.New(os.Getenv("QUICKNODE_API_KEY"))
	if err != nil {
		return fmt.Errorf("could not build a client to confirm the destroy: %w", err)
	}
	ctx := context.Background()

	for name, resourceState := range state.RootModule().Resources {
		attributes := resourceState.Primary.Attributes
		switch resourceState.Type {
		case "quicknode_kv_list":
			items, err := quicknode.GetKVList(ctx, attributes["key"])
			if err != nil {
				return fmt.Errorf("%s: could not confirm list %s was destroyed: %w", name, attributes["key"], err)
			}
			if len(items) > 0 {
				return fmt.Errorf("%s: list %s still has %d items after destroy", name, attributes["key"], len(items))
			}
		case "quicknode_kv_list_items":
			items, err := quicknode.GetKVList(ctx, attributes["list_key"])
			if err != nil {
				return fmt.Errorf("%s: could not confirm the items were removed from %s: %w", name, attributes["list_key"], err)
			}
			for key, item := range attributes {
				if strings.HasPrefix(key, "items.") && key != "items.#" && slices.Contains(items, item) {
					return fmt.Errorf("%s: list %s still holds %s after destroy", name, attributes["list_key"], item)
				}
			}
		case "quicknode_kv_value":
			_, err := quicknode.GetKVValue(ctx, attributes["key"])
			if client.IsNotFound(err) {
				continue
			}
			if err != nil {
				return fmt.Errorf("%s: could not confirm value %s was destroyed: %w", name, attributes["key"], err)
			}
			return fmt.Errorf("%s: value %s still exists after destroy", name, attributes["key"])
		}
	}
	return nil
}

func TestAccKVList_lifecycle(t *testing.T) {
	key := kvAcceptanceKey()
	withItems := func(items string) string {
		return fmt.Sprintf(`
resource "quicknode_kv_list" "test" {
  key   = %q
  items = %s
}

data "quicknode_kv_list" "test" {
  key = quicknode_kv_list.test.key
  depends_on = [quicknode_kv_list.test]
}
`, key, items)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckKVDestroyed,
		Steps: []resource.TestStep{
			{
				Config: withItems(`["0xAbC", "0xabc", "b"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("quicknode_kv_list.test", "id", key),
					resource.TestCheckResourceAttr("quicknode_kv_list.test", "items.#", "3"),
					resource.TestCheckTypeSetElemAttr("quicknode_kv_list.test", "items.*", "0xAbC"),
					resource.TestCheckResourceAttr("data.quicknode_kv_list.test", "items.#", "3"),
				),
			},
			{
				ResourceName:      "quicknode_kv_list.test",
				ImportState:       true,
				ImportStateId:     key,
				ImportStateVerify: true,
			},
			{
				Config: withItems(`["0xabc", "c", "d"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("quicknode_kv_list.test", "items.#", "3"),
					resource.TestCheckTypeSetElemAttr("quicknode_kv_list.test", "items.*", "c"),
					resource.TestCheckResourceAttr("data.quicknode_kv_list.test", "items.#", "3"),
					resource.TestCheckTypeSetElemAttr("data.quicknode_kv_list.test", "items.*", "d"),
				),
			},
		},
	})
}

// TestAccKVList_refusesAnExistingList checks that creating a list never merges
// into one Terraform does not own.
func TestAccKVList_refusesAnExistingList(t *testing.T) {
	key := kvAcceptanceKey()
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckKVDestroyed,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					quicknode := testAccClient(t)
					if err := quicknode.CreateKVList(context.Background(), key, []string{"outside"}); err != nil {
						t.Fatalf("could not create the existing list: %v", err)
					}
					t.Cleanup(func() { _ = quicknode.DeleteKVList(context.Background(), key) })
				},
				Config: fmt.Sprintf(`
resource "quicknode_kv_list" "test" {
  key   = %q
  items = ["inside"]
}
`, key),
				ExpectError: regexp.MustCompile(`Key-value list already exists`),
			},
		},
	})
}

func kvListItemsConfig(key, items string) string {
	return fmt.Sprintf(`
resource "quicknode_kv_list_items" "test" {
  list_key = %q
  items    = %s
}

data "quicknode_kv_list" "test" {
  key        = quicknode_kv_list_items.test.list_key
  depends_on = [quicknode_kv_list_items.test]
}
`, key, items)
}

func TestAccKVListItems_leavesOtherItemsAlone(t *testing.T) {
	key := kvAcceptanceKey()
	t.Cleanup(func() { _ = testAccClient(t).DeleteKVList(context.Background(), key) })
	const wallet = "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckKVDestroyed,
		Steps: []resource.TestStep{
			{
				Config: kvListItemsConfig(key, fmt.Sprintf(`[%q, "tokens/usdc"]`, wallet)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("quicknode_kv_list_items.test", "id", key),
					resource.TestCheckResourceAttr("quicknode_kv_list_items.test", "items.#", "2"),
					func(*terraform.State) error {
						return testAccClient(t).UpdateKVList(context.Background(), key, []string{"added-outside"}, nil)
					},
				),
			},
			{
				Config: kvListItemsConfig(key, fmt.Sprintf(`[%q, "0xnew"]`, wallet)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("quicknode_kv_list_items.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("quicknode_kv_list_items.test", "items.#", "2"),
					resource.TestCheckResourceAttr("data.quicknode_kv_list.test", "items.#", "3"),
					resource.TestCheckTypeSetElemAttr("data.quicknode_kv_list.test", "items.*", "added-outside"),
					resource.TestCheckTypeSetElemAttr("data.quicknode_kv_list.test", "items.*", "0xnew"),
				),
			},
			{
				PreConfig: func() {
					if err := testAccClient(t).UpdateKVList(context.Background(), key, nil, []string{wallet}); err != nil {
						t.Fatalf("could not remove the item outside Terraform: %v", err)
					}
				},
				Config: kvListItemsConfig(key, fmt.Sprintf(`[%q, "0xnew"]`, wallet)),
				Check:  resource.TestCheckTypeSetElemAttr("data.quicknode_kv_list.test", "items.*", wallet),
			},
			{
				Config: `# empty`,
				Check: func(*terraform.State) error {
					items, err := testAccClient(t).GetKVList(context.Background(), key)
					if err != nil {
						return err
					}
					if len(items) != 1 || items[0] != "added-outside" {
						return fmt.Errorf("list holds %v after destroy, want only the item added outside Terraform", items)
					}
					return nil
				},
			},
		},
	})
}

// TestAccKVListItems_takesOverExistingItemsByImport checks that create refuses
// items already in the list, and that importing then applying takes over only
// the configured ones.
func TestAccKVListItems_takesOverExistingItemsByImport(t *testing.T) {
	key := kvAcceptanceKey()
	t.Cleanup(func() { _ = testAccClient(t).DeleteKVList(context.Background(), key) })
	config := fmt.Sprintf(`
resource "quicknode_kv_list_items" "test" {
  list_key = %q
  items    = ["owned"]
}
`, key)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckKVDestroyed,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					if err := testAccClient(t).CreateKVList(context.Background(), key, []string{"owned", "kept"}); err != nil {
						t.Fatalf("could not create the existing list: %v", err)
					}
				},
				Config:      config,
				ExpectError: regexp.MustCompile(`Items are already in the list`),
			},
			{
				Config:             config,
				ResourceName:       "quicknode_kv_list_items.test",
				ImportState:        true,
				ImportStateId:      key,
				ImportStatePersist: true,
			},
			{
				Config: config,
				Check:  resource.TestCheckResourceAttr("quicknode_kv_list_items.test", "items.#", "1"),
			},
			{
				Config: `# empty`,
				Check: func(*terraform.State) error {
					items, err := testAccClient(t).GetKVList(context.Background(), key)
					if err != nil {
						return err
					}
					if len(items) != 1 || items[0] != "kept" {
						return fmt.Errorf("list holds %v after destroy, want only the item Terraform never owned", items)
					}
					return nil
				},
			},
		},
	})
}

func TestAccKVValue_lifecycle(t *testing.T) {
	key := kvAcceptanceKey()
	withValue := func(value string) string {
		return fmt.Sprintf(`
resource "quicknode_kv_value" "test" {
  key   = %q
  value = %s
}

data "quicknode_kv_value" "test" {
  key        = quicknode_kv_value.test.key
  depends_on = [quicknode_kv_value.test]
}
`, key, value)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckKVDestroyed,
		Steps: []resource.TestStep{
			{
				Config: withValue(`"100"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("quicknode_kv_value.test", "value", "100"),
					resource.TestCheckResourceAttr("data.quicknode_kv_value.test", "value", "100"),
				),
			},
			{
				ResourceName:      "quicknode_kv_value.test",
				ImportState:       true,
				ImportStateId:     key,
				ImportStateVerify: true,
			},
			{
				Config: withValue(`jsonencode({ min = 1, max = 5 })`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("quicknode_kv_value.test", "value", `{"max":5,"min":1}`),
					resource.TestCheckResourceAttr("data.quicknode_kv_value.test", "value", `{"max":5,"min":1}`),
				),
			},
		},
	})
}
