resource "quicknode_endpoint" "api" {
  chain      = "eth"
  network    = "mainnet"
  multichain = false
  status     = "active"
}

# A bucket set here overrides the plan default. A bucket left out keeps it, and
# removing one that was set returns that bucket to the plan default.
resource "quicknode_endpoint_rate_limits" "api" {
  endpoint_id = quicknode_endpoint.api.id

  rps = 25
  rpd = 1000000
}

output "plan_allows_per_second" {
  value = quicknode_endpoint_rate_limits.api.plan_default.rps
}
