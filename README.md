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
  chain   = "ethereum"
  network = "mainnet"
  label   = "payments-prod"
}

output "rpc_url" {
  value     = quicknode_endpoint.payments.http_url
  sensitive = true # the URL embeds your auth token
}
```

Authentication uses a Quicknode [Admin API](https://www.quicknode.com/docs/admin-api) key (paid plans), via the `QUICKNODE_API_KEY` environment variable or the provider block. Endpoint URLs contain auth tokens and are stored in Terraform state — use encrypted remote state.

## Resources

| Name | Status |
|---|---|
| `quicknode_endpoint` | ✅ available |
| Endpoint security (IP / domain allowlists, method filters) | ✅ available (being redesigned) |
| `quicknode_endpoint_rate_limits`, `quicknode_method_rate_limit` | 🚧 planned |
| `quicknode_stream`, `quicknode_webhook` | 🚧 planned |
| Data sources: `quicknode_chains`, `quicknode_endpoint(s)` | ✅ available |

Full documentation lives in [`docs/`](./docs) and, once published, on the Terraform Registry.

## Development

Requires Go (see `go.mod`) and Terraform >= 1.13.

```sh
make generate   # regenerate the API client and docs
make lint       # golangci-lint
go build ./...
TF_ACC=1 go test ./... # acceptance tests: creates real, billable resources
```

Contributions welcome — see [CONTRIBUTING.md](./CONTRIBUTING.md).

## License

[MPL-2.0](./LICENSE)
