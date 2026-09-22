resource "quicknode_endpoint" "api" {
  chain   = "eth"
  network = "mainnet"

  security_options = {
    tokens = true
  }
}

# Every endpoint is created with one token. This adds a second, so a credential
# can be handed to one consumer and revoked later without disturbing the rest.
resource "quicknode_endpoint_token" "indexer" {
  endpoint_id = quicknode_endpoint.api.id
}

output "indexer_token" {
  value     = quicknode_endpoint_token.indexer.token
  sensitive = true
}
