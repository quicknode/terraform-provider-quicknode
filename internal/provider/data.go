package provider

import "github.com/quicknode/terraform-provider-quicknode/internal/client"

// providerData is what Configure hands to every resource and data source.
// Chains is fetched once when the provider configures so that resources can
// validate chain and network slugs during plan, before anything is created.
type providerData struct {
	Client *client.Client
	Chains []client.Chain
}
