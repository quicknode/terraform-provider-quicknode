resource "quicknode_endpoint" "api" {
  chain   = "eth"
  network = "mainnet"
}

# Keep a few expensive calls from consuming the endpoint's whole budget.
resource "quicknode_endpoint_method_rate_limit" "heavy_reads" {
  endpoint_id = quicknode_endpoint.api.id

  methods  = ["eth_getLogs", "debug_traceTransaction"]
  rate     = 5
  interval = "second"
}

# Disabling keeps the definition in place, which suits turning a limit off
# during an incident without losing it.
resource "quicknode_endpoint_method_rate_limit" "trace_block" {
  endpoint_id = quicknode_endpoint.api.id

  methods  = ["trace_block"]
  rate     = 100
  interval = "minute"
  enabled  = false
}
