data "quicknode_chains" "all" {}

# Chain slugs are abbreviations that often differ from the chain's name, so look
# one up instead of hardcoding it.
locals {
  ethereum = one([for chain in data.quicknode_chains.all.chains : chain if chain.slug == "eth"])
}

resource "quicknode_endpoint" "primary" {
  chain      = local.ethereum.slug
  network    = one([for network in local.ethereum.networks : network.slug if network.chain_id == 1])
  multichain = false
  status     = "active"
}
