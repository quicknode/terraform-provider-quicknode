# Terraform Provider for Quicknode

The official [Terraform](https://developer.hashicorp.com/terraform)-compatible
provider for [Quicknode](https://www.quicknode.com). Manage RPC endpoints,
security rules and rate limits with `terraform plan` and `terraform apply`.

- [Provider documentation](./docs) — also published to the Terraform Registry
- [Contributing](./CONTRIBUTING.md)
- [Changelog](./CHANGELOG.md)

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/install) 1.13 or later
- [Go](https://go.dev/doc/install) — the version in [`go.mod`](./go.mod), to build from source

## Using the provider

```hcl
terraform {
  required_providers {
    quicknode = {
      source  = "quicknode/quicknode"
      version = "~> 0.3.0"
    }
  }
}

provider "quicknode" {}

resource "quicknode_endpoint" "payments" {
  chain      = "eth"
  network    = "mainnet"
  label      = "payments-prod"
  status     = "active"
  multichain = false
}
```

Authentication uses a Quicknode [API key](https://www.quicknode.com/docs/admin-api),
available on paid plans. Set `QUICKNODE_API_KEY` in the environment; do not
write it into a configuration file.

The provider covers endpoints, the security mechanisms they enforce and who is
allowed past them, RPC method filtering, and rate limits both endpoint-wide and
per method. Endpoints created elsewhere are readable through
`data.quicknode_endpoint` and `data.quicknode_endpoints`.

A security mechanism is enabled on the endpoint and the entries it applies to
are separate resources, so an entry added outside Terraform is left alone rather
than deleted on the next apply.

Full resource and attribute reference lives in [`docs/`](./docs).

## Building

```sh
make build
```

## Developing

To run Terraform against a local build, add a development override to
`~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "quicknode/quicknode" = "/path/to/your/GOPATH/bin"
  }
  direct {}
}
```

Then `go install .` and run Terraform normally. With an override in place,
`terraform init` is neither needed nor supported.

```sh
make lint   # gofmt, go vet, golangci-lint
make docs   # regenerate docs/ from the provider schema and examples/
make fmt    # format Go and Terraform sources
```

`docs/` is generated. CI fails if it is out of date with the schema, so run
`make docs` after any schema change and commit the result.

The API client in `api/admin/` is generated and should not be edited by hand.
See [CONTRIBUTING.md](./CONTRIBUTING.md#making-changes) for how to refresh it.

## Testing

Unit tests need no credentials:

```sh
make test
```

Acceptance tests create real, billable Quicknode resources and require an Admin
API key for a paid account dedicated to testing:

```sh
QUICKNODE_API_KEY=... make testacc
```

## Security

Endpoint tokens and credentialed URLs are written to Terraform state. Use
encrypted remote state and treat state files as credential material. See
[SECURITY.md](./SECURITY.md) to report a vulnerability.

## License

[MPL-2.0](./LICENSE)
