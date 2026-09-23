resource "quicknode_endpoint" "api" {
  chain   = "eth"
  network = "mainnet"

  security_options = {
    referrers = true
  }
}

resource "quicknode_endpoint_referrer" "app" {
  endpoint_id = quicknode_endpoint.api.id
  referrer    = "https://app.example.com"
}
