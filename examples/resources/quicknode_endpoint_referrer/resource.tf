resource "quicknode_endpoint" "api" {
  chain      = "eth"
  network    = "mainnet"
  multichain = false
  status     = "active"

  security_options = {
    referrers = true
  }
}

resource "quicknode_endpoint_referrer" "app" {
  endpoint_id = quicknode_endpoint.api.id
  referrer    = "https://app.example.com"
}
