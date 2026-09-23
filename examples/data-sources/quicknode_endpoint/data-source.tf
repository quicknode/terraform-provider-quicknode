# Look up an endpoint created outside Terraform, by id or by label.
data "quicknode_endpoint" "payments" {
  label = "payments-prod"
}

output "payments_rpc_url" {
  value     = data.quicknode_endpoint.payments.http_url_with_token
  sensitive = true
}

# Attach an allowlist entry to an endpoint this configuration does not own.
resource "quicknode_endpoint_ip" "office" {
  endpoint_id = data.quicknode_endpoint.payments.id
  ip          = "203.0.113.7"
}
