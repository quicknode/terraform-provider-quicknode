terraform {
  required_providers {
    quicknode = {
      source = "quicknode/quicknode"
    }
  }
}

# Reads QUICKNODE_API_KEY from the environment.
provider "quicknode" {}

data "quicknode_chains" "all" {}

locals {
  ethereum = one([for chain in data.quicknode_chains.all.chains : chain if chain.slug == "eth"])
}

resource "quicknode_endpoint" "payments" {
  chain   = local.ethereum.slug
  network = "mainnet"
  label   = "payments-prod"
  status  = "active"
  tags    = ["prod", "payments"]
}

output "rpc_url" {
  value     = quicknode_endpoint.payments.http_url_with_token
  sensitive = true
}

output "rpc_base_url" {
  value = quicknode_endpoint.payments.http_url
}
