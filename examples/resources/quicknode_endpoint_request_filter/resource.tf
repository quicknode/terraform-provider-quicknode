resource "quicknode_endpoint" "api" {
  chain   = "eth"
  network = "mainnet"
}

# Anything outside the set is rejected. security_options.request_filters on the
# endpoint flips to true on its own once a filter exists.
resource "quicknode_endpoint_request_filter" "read_only" {
  endpoint_id = quicknode_endpoint.api.id

  methods = [
    "eth_blockNumber",
    "eth_call",
    "eth_getBalance",
    "eth_getLogs",
  ]
}
