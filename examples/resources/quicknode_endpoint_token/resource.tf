resource "quicknode_endpoint" "api" {
  chain      = "eth"
  network    = "mainnet"
  multichain = false
  status     = "active"

  security_options = {
    tokens = true
  }
}

# Every endpoint is created with one token. This adds a second, so a credential
# can be handed to one consumer and revoked later without disturbing the rest.
resource "quicknode_endpoint_token" "indexer" {
  endpoint_id = quicknode_endpoint.api.id
}

# The endpoint's URL carrying this token, for the consumer it was issued to.
output "indexer_rpc_url" {
  value     = quicknode_endpoint_token.indexer.http_url_with_token
  sensitive = true
}
