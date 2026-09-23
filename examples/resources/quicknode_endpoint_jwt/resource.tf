resource "quicknode_endpoint" "api" {
  chain   = "eth"
  network = "mainnet"

  security_options = {
    jwts   = true
    tokens = false
  }
}

resource "quicknode_endpoint_jwt" "signer" {
  endpoint_id = quicknode_endpoint.api.id
  name        = "signer"
  public_key  = file("${path.module}/signer.pub.pem")
}

# Put the generated kid in the header of the tokens signed with the matching
# private key.
output "jwt_kid" {
  value = quicknode_endpoint_jwt.signer.kid
}
