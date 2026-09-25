resource "quicknode_endpoint" "api" {
  chain      = "eth"
  network    = "mainnet"
  multichain = false
  status     = "active"

  security_options = {
    ips = true
  }
}

resource "quicknode_endpoint_ip" "office" {
  endpoint_id = quicknode_endpoint.api.id
  ip          = "203.0.113.7"
}

# Entries may be added before the toggle is enabled, which is the safe order for
# an endpoint already serving traffic. Adding one while security_options.ips is
# false produces a warning and not an error.
resource "quicknode_endpoint_ip" "vpn" {
  endpoint_id = quicknode_endpoint.api.id
  ip          = "198.51.100.0/24"
}
