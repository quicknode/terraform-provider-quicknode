OAPI_CODEGEN_VERSION ?= v2.8.0
TFPLUGINDOCS_VERSION ?= v0.23.0
GOLANGCI_LINT_VERSION ?= v2.5.0
ADMIN_SPEC_URL ?= https://www.quicknode.com/docs/openapi/admin-api.openapi.json

default: build

build:
	go build ./...

# vendor refreshes the upstream Admin API spec. Review the diff before
# generating: an upstream change can silently invalidate a patch in
# api/admin/patches.json.
vendor:
	curl -sSf -o api/admin/openapi.upstream.json $(ADMIN_SPEC_URL)

generate:
	go run ./api/admin/gen
	cd api/admin && go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION) -config config.yaml openapi.json
	gofmt -w api/admin/admin.gen.go

docs:
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@$(TFPLUGINDOCS_VERSION) generate --provider-name quicknode

fmt:
	gofmt -w .
	terraform fmt -recursive ./examples/

lint:
	gofmt -l . | tee /dev/stderr | (! read)
	go vet ./...
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...

test:
	go test ./...

# testacc creates real, billable Quicknode resources. It needs QUICKNODE_API_KEY
# for a paid account that is dedicated to testing.
testacc:
	TF_ACC=1 go test ./... -v -timeout 30m

.PHONY: default build vendor generate docs fmt lint test testacc
