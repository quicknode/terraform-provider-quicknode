resource "quicknode_endpoint" "api" {
  chain      = "eth"
  network    = "mainnet"
  multichain = true
  status     = "active"
}

# depends_on defers the read to the apply, so it sees multichain enabled.
data "quicknode_endpoint_urls" "api" {
  endpoint_id = quicknode_endpoint.api.id
  depends_on  = [quicknode_endpoint.api]
}

# Pass the credentialed URL to whatever makes RPC calls.
output "rpc_url" {
  value     = data.quicknode_endpoint_urls.api.http_url_with_token
  sensitive = true
}

# The same endpoint on another network, keyed by network slug.
output "base_rpc_url" {
  value     = data.quicknode_endpoint_urls.api.multichain_urls_with_token["base-mainnet"].http_url
  sensitive = true
}

# Every network the endpoint serves, safe to log or display.
output "networks" {
  value = keys(data.quicknode_endpoint_urls.api.safe_multichain_urls)
}
