resource "quicknode_endpoint" "api" {
  chain      = "eth"
  network    = "mainnet"
  multichain = false
  status     = "active"

  security_options = {
    jwts   = true
    tokens = false
  }
}

resource "quicknode_endpoint_jwt" "signer" {
  endpoint_id = quicknode_endpoint.api.id
  name        = "signer"
  kid         = "signer-2026-01"
  public_key  = file("${path.module}/signer.pub.pem")
}

# Tokens signed with the matching private key carry the same kid in their
# header, which is how Quicknode picks the key to verify them with.
