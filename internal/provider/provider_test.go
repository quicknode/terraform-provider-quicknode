package provider_test

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/quicknode/terraform-provider-quicknode/internal/provider"
)

func TestProviderSchema(t *testing.T) {
	server := providerserver.NewProtocol6(provider.New("test")())()

	schema, err := server.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("GetProviderSchema: %v", err)
	}
	for _, diagnostic := range schema.Diagnostics {
		if diagnostic.Severity == tfprotov6.DiagnosticSeverityError {
			t.Errorf("schema diagnostic: %s — %s", diagnostic.Summary, diagnostic.Detail)
		}
	}

	for _, name := range []string{
		"quicknode_endpoint",
		"quicknode_endpoint_ip",
		"quicknode_endpoint_domain_mask",
		"quicknode_endpoint_referrer",
		"quicknode_endpoint_jwt",
		"quicknode_endpoint_request_filter",
		"quicknode_endpoint_token",
	} {
		if _, ok := schema.ResourceSchemas[name]; !ok {
			t.Errorf("%s is missing, got %v", name, keys(schema.ResourceSchemas))
		}
	}
	if _, ok := schema.DataSourceSchemas["quicknode_chains"]; !ok {
		t.Errorf("quicknode_chains is missing, got %v", keys(schema.DataSourceSchemas))
	}
}

func keys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
