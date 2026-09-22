resource "quicknode_endpoint" "payments" {
  chain   = "eth"
  network = "mainnet"
  label   = "payments-prod"
  status  = "active"
  tags    = ["prod", "payments"]
}

# Pass the credentialed URL to whatever makes RPC calls.
output "payments_rpc_url" {
  value     = quicknode_endpoint.payments.http_url_with_token
  sensitive = true
}

# The same endpoint without the credential, safe to log or display.
output "payments_rpc_host" {
  value = quicknode_endpoint.payments.http_url
}
