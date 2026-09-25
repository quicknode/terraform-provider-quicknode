resource "quicknode_endpoint" "api" {
  chain      = "eth"
  network    = "mainnet"
  multichain = false
  status     = "active"
}

# Anything outside the set is rejected. The Admin API turns filtering on once a
# filter exists.
resource "quicknode_endpoint_request_filter" "read_only" {
  endpoint_id = quicknode_endpoint.api.id

  methods = [
    "eth_blockNumber",
    "eth_call",
    "eth_getBalance",
    "eth_getLogs",
  ]
}
