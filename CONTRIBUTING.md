# Contributing

Thanks for helping improve the Quicknode Terraform provider.

## Getting set up

You need Go (the version in `go.mod`) and Terraform 1.13 or later.

```sh
make build   # compile
make test    # unit tests
make lint    # gofmt, go vet, golangci-lint
```

## Trying a local build

Terraform normally downloads providers from the registry. To point it at a local
build instead, add a development override to `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "quicknode/quicknode" = "/path/to/your/GOPATH/bin"
  }
  direct {}
}
```

Then `go install .` and run Terraform as usual. Skip `terraform init` — with an
override in place it is neither needed nor supported.

## Making changes

**Schema changes require regenerated docs.** `docs/` is generated from the
provider schema and the files under `examples/`, and CI fails if it is stale:

```sh
make docs
```

Commit the result alongside your change.

**API client changes.** `api/admin/admin.gen.go` is generated and should never
be edited by hand. It comes from `api/admin/openapi.upstream.json`, which is
vendored verbatim, plus the local schema corrections in
`api/admin/patches.json`. To pick up an upstream spec change:

```sh
make vendor     # refresh the vendored spec
make generate   # apply patches, regenerate the client
```

Review the `make vendor` diff before generating — an upstream change can
silently invalidate a patch.

Domain types live in `internal/client`, not in the generated package. The
provider layer imports `internal/client` and never `api/admin`.

## Tests

Unit tests run against `httptest` servers and need no credentials.

Acceptance tests create real, billable Quicknode resources. They need an Admin
API key for a paid account dedicated to testing:

```sh
QUICKNODE_API_KEY=... make testacc
```

Do not run them against an account with production endpoints.

## Pull requests

- One logical change per pull request.
- Include tests for behavior changes.
- Run `make lint`, `make test` and `make docs` before pushing.
- Describe what the change does and why, not just what files moved.

## Reporting bugs

Open an issue with the provider version, the Terraform version, a minimal
configuration that reproduces the problem, and the output of the failing
command. Redact API keys and endpoint URLs containing tokens.
