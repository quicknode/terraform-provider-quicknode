# Terraform Provider for Quicknode

The official [Terraform](https://developer.hashicorp.com/terraform) provider for [Quicknode](https://www.quicknode.com) — manage your blockchain infrastructure as code: RPC endpoints, security rules, rate limits, Streams, and Webhooks, all through `terraform plan` and `apply`.

## Why

Teams run their Quicknode setup across environments and chains, and that setup deserves the same workflow as the rest of their infrastructure: version control, pull-request review for security changes, drift detection, and repeatable environments — no click-ops.

## Usage

```hcl
terraform {
  required_providers {
    quicknode = {
      source  = "quicknode/quicknode"
      version = "~> 0.1"
    }
  }
}

# Reads QUICKNODE_API_KEY from the environment
provider "quicknode" {}

resource "quicknode_endpoint" "payments" {
  chain   = "eth"
  network = "mainnet"
  label   = "payments-prod"
  tags    = ["prod", "payments"]
}

# Working endpoint, credential included.
output "rpc_url" {
  value     = quicknode_endpoint.payments.http_url_with_token
  sensitive = true
}

# Same endpoint with the credential removed: safe to log or display.
output "rpc_base_url" {
  value = quicknode_endpoint.payments.http_url
}
```

Chain slugs are abbreviations that often differ from the chain's name — Ethereum is `eth`, Avalanche is `avax`, Arbitrum is `arb`, Polygon is `matic`. Read `data.quicknode_chains` for the full list.

Authentication uses a Quicknode [Admin API](https://www.quicknode.com/docs/admin-api) key (paid plans), via the `QUICKNODE_API_KEY` environment variable or the provider block.

The Admin API returns endpoint URLs with the auth token embedded. The provider exposes both forms:

| Attribute | Sensitive | Use it for |
|---|---|---|
| `http_url_with_token`, `wss_url_with_token` | yes | anything that makes RPC calls |
| `http_url`, `wss_url` | no | logging, display, anything that must not hold a credential |
| `tokens` | yes | rotation workflows that need token ids |

Do not rebuild a URL by joining `http_url` to a token. The token is not always the last path segment — some chains append a suffix, as in `https://<host>/<token>/evm` — so a hand-assembled URL works on Ethereum and breaks elsewhere. `wss_url` is null on chains without WebSocket support.

Token values land in Terraform state either way — use encrypted remote state.

## Resources

| Name | Status |
|---|---|
| `quicknode_endpoint` (label, status, tags, multichain, import) | ✅ available |
| `data.quicknode_chains` | ✅ available |
| `security_options` on `quicknode_endpoint` | 🚧 planned |
| `quicknode_endpoint_ip`, `quicknode_endpoint_domain_mask`, `quicknode_endpoint_referrer`, `quicknode_endpoint_jwt`, `quicknode_endpoint_request_filter` | 🚧 planned |
| `quicknode_endpoint_rate_limits`, `quicknode_method_rate_limit` | 🚧 planned |
| `data.quicknode_endpoint(s)` | 🚧 planned |
| `quicknode_stream`, `quicknode_webhook` | 🚧 planned, blocked on published specs |

Security splits along the way the API splits. The toggles are a single PATCH on
the endpoint, so they become a `security_options` attribute on
`quicknode_endpoint`. Each allowlist entry is a POST/DELETE with no update, so
each becomes its own resource keyed by `endpoint_id` — which also keeps a
one-address change from showing up as a diff on the whole endpoint.

Full documentation lives in [`docs/`](./docs) and, once published, on the Terraform Registry.

## Development

Requires Go (see `go.mod`) and Terraform >= 1.13.

```sh
make vendor     # refresh the upstream Admin API spec
make generate   # apply api/admin/patches.json, then regenerate the client
make lint       # gofmt + go vet
make test       # unit tests
make testacc    # acceptance tests: creates real, billable resources
```

The client is generated from `api/admin/openapi.upstream.json`, which is vendored
verbatim so `make vendor` can refresh it. `api/admin/patches.json` holds the local
schema corrections applied before generation, keeping the vendored copy unmodified.

Contributions welcome — see [CONTRIBUTING.md](./CONTRIBUTING.md).

## License

[MPL-2.0](./LICENSE)
