resource "quicknode_endpoint" "payments" {
  chain      = "eth"
  network    = "mainnet"
  label      = "payments-prod"
  status     = "active"
  multichain = false
  tags       = ["prod", "payments"]

  # Each toggle decides whether a mechanism is enforced. The entries it applies
  # to are separate resources, such as quicknode_endpoint_ip. A toggle left out
  # keeps whatever value the endpoint already has.
  security_options = {
    tokens = true
    ips    = true
    cors   = false
  }

  # Read the caller's address from this header when calls arrive through a
  # proxy, so IP restrictions match the original caller.
  ip_custom_header = "X-Real-IP"
}

# The URL with the credential replaced by the literal REPLACE_WITH_TOKEN. Safe
# to log or display, and it keeps the real URL's shape, so substituting a token
# reproduces a working address on every chain.
output "payments_rpc_url_redacted" {
  value = quicknode_endpoint.payments.safe_http_url
}

# The credentialed URL is read from data.quicknode_endpoint_urls, because it
# changes whenever the endpoint's tokens do.
data "quicknode_endpoint_urls" "payments" {
  endpoint_id = quicknode_endpoint.payments.id
}

output "payments_rpc_url" {
  value     = data.quicknode_endpoint_urls.payments.http_url_with_token
  sensitive = true
}
