# Several values in one filter match any of them; several filters must all
# match. Every page is walked, so this is the whole account.
data "quicknode_endpoints" "production" {
  tag_labels = ["prod"]
  statuses   = ["active"]
}

# Apply the same rate limit across every matching endpoint.
resource "quicknode_endpoint_rate_limits" "production" {
  for_each = toset(data.quicknode_endpoints.production.ids)

  endpoint_id = each.value
  rps         = 50
}

output "production_networks" {
  value = [for endpoint in data.quicknode_endpoints.production.endpoints : endpoint.network]
}
