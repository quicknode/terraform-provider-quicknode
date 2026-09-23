resource "quicknode_endpoint" "api" {
  chain   = "eth"
  network = "mainnet"

  security_options = {
    domain_masks = true
  }
}

resource "quicknode_endpoint_domain_mask" "rpc" {
  endpoint_id = quicknode_endpoint.api.id
  domain      = "rpc.example.com"
}
