package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// securityBody mirrors a live GET /v0/endpoints/{id}/security response. The
// list entries are the shapes the patched spec declares, which the published
// spec leaves as untyped arrays.
const securityBody = `{"data":{
"options":{"tokens":true,"referrers":false,"jwts":false,"ips":true,"domainMasks":false,
"hsts":true,"cors":false,"requestFilters":true,"responseLogging":false,
"ipCustomHeader":{"value":"X-Real-IP"}},
"ips":[{"id":"ip-1","ip":"203.0.113.7"}],
"domain_masks":[{"id":"dm-1","domain":"rpc.example.com"}],
"referrers":[{"id":"rf-1","referrer":"https://example.com"}],
"jwts":[{"id":"jwt-1","name":"signer","kid":"kid-1","public_key":"-----BEGIN PUBLIC KEY-----"}],
"request_filters":[{"id":"filter-1","method":["eth_call","eth_getLogs"]}],
"tokens":[{"id":"tok-1","token":"TOKENVALUE"}]}}`

func TestGetEndpointSecurity(t *testing.T) {
	client := newTestClient(t, securityBody)

	security, err := client.GetEndpointSecurity(context.Background(), "1")
	if err != nil {
		t.Fatalf("GetEndpointSecurity: %v", err)
	}

	if !security.Options.Tokens || !security.Options.IPs || !security.Options.HSTS {
		t.Errorf("enabled toggles did not survive: %+v", security.Options)
	}
	if security.Options.Referrers || security.Options.Cors || security.Options.ResponseLogging {
		t.Errorf("disabled toggles did not survive: %+v", security.Options)
	}
	if !security.Options.RequestFilters {
		t.Error("requestFilters is read-only but still has to be reported")
	}
	if security.Options.IPCustomHeader != "X-Real-IP" {
		t.Errorf("IPCustomHeader = %q, want X-Real-IP", security.Options.IPCustomHeader)
	}

	if len(security.IPs) != 1 || security.IPs[0].ID != "ip-1" || security.IPs[0].Value != "203.0.113.7" {
		t.Errorf("IPs = %+v", security.IPs)
	}
	if len(security.DomainMasks) != 1 || security.DomainMasks[0].Value != "rpc.example.com" {
		t.Errorf("DomainMasks = %+v", security.DomainMasks)
	}
	if len(security.Referrers) != 1 || security.Referrers[0].Value != "https://example.com" {
		t.Errorf("Referrers = %+v", security.Referrers)
	}
	if len(security.JWTs) != 1 || security.JWTs[0].KID != "kid-1" || security.JWTs[0].Name != "signer" {
		t.Errorf("JWTs = %+v", security.JWTs)
	}
	if len(security.RequestFilters) != 1 || len(security.RequestFilters[0].Methods) != 2 {
		t.Fatalf("RequestFilters = %+v", security.RequestFilters)
	}
	if security.RequestFilters[0].Methods[0] != "eth_call" {
		t.Errorf("first filtered method = %q", security.RequestFilters[0].Methods[0])
	}
	if len(security.Tokens) != 1 || security.Tokens[0].Value != "TOKENVALUE" {
		t.Errorf("Tokens = %+v", security.Tokens)
	}
}

// TestSetSecurityOptionsSendsStrings guards the asymmetry between the two
// halves of the API: reads report the toggles as booleans and the write takes
// the strings "enabled" and "disabled". A toggle the configuration does not
// manage has to stay out of the body entirely rather than being sent as false.
func TestSetSecurityOptionsSendsStrings(t *testing.T) {
	var captured map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"option":"tokens","status":"enabled"},{"option":"cors","status":"disabled"}]}`))
	}))
	t.Cleanup(server.Close)

	client, err := New("test-key", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	enabled, disabled := true, false
	err = client.SetSecurityOptions(context.Background(), "1", SecurityOptionsPatch{
		Tokens: &enabled,
		Cors:   &disabled,
	})
	if err != nil {
		t.Fatalf("SetSecurityOptions: %v", err)
	}

	options, ok := captured["options"].(map[string]any)
	if !ok {
		t.Fatalf("request carried no options object: %v", captured)
	}
	if options["tokens"] != "enabled" {
		t.Errorf("tokens = %v, want the string \"enabled\"", options["tokens"])
	}
	if options["cors"] != "disabled" {
		t.Errorf("cors = %v, want the string \"disabled\"", options["cors"])
	}
	for _, unmanaged := range []string{"jwts", "ips", "hsts", "referrers", "domainMasks"} {
		if _, present := options[unmanaged]; present {
			t.Errorf("%s was sent although the patch left it unset", unmanaged)
		}
	}
}

func TestRemoveEndpointIPRejectsFalseResult(t *testing.T) {
	client := newTestClient(t, `{"data":false}`)

	err := client.RemoveEndpointIP(context.Background(), "1", "ip-1")
	if err == nil {
		t.Fatal("a data:false delete has to be an error, not a silent success")
	}
}

func TestRemoveEndpointIPAcceptsTrueResult(t *testing.T) {
	client := newTestClient(t, `{"data":true}`)

	if err := client.RemoveEndpointIP(context.Background(), "1", "ip-1"); err != nil {
		t.Fatalf("RemoveEndpointIP: %v", err)
	}
}

// TestDeleteAbsentEntriesSucceed covers the routes a Terraform destroy hits
// after something already removed the entry out of band. The desired state is
// "gone", so a 404 is not a failure.
func TestDeleteAbsentEntriesSucceed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	client, err := New("test-key", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	for name, remove := range map[string]func() error{
		"ip":             func() error { return client.RemoveEndpointIP(ctx, "1", "ip-1") },
		"domain mask":    func() error { return client.RemoveDomainMask(ctx, "1", "dm-1") },
		"referrer":       func() error { return client.RemoveReferrer(ctx, "1", "rf-1") },
		"jwt":            func() error { return client.RemoveJWT(ctx, "1", "jwt-1") },
		"request filter": func() error { return client.RemoveRequestFilter(ctx, "1", "filter-1") },
		"token":          func() error { return client.RemoveEndpointToken(ctx, "1", "tok-1") },
		"custom header":  func() error { return client.DeleteIPCustomHeader(ctx, "1") },
	} {
		if err := remove(); err != nil {
			t.Errorf("removing an absent %s should succeed, got %v", name, err)
		}
	}
}

// TestRequestFilterNoContentRoutes covers the two routes that answer 204 with an
// empty body instead of the JSON envelope every other route uses.
func TestRequestFilterNoContentRoutes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client, err := New("test-key", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	if err := client.UpdateRequestFilter(ctx, "1", "filter-1", []string{"eth_call"}); err != nil {
		t.Errorf("UpdateRequestFilter: %v", err)
	}
	if err := client.RemoveRequestFilter(ctx, "1", "filter-1"); err != nil {
		t.Errorf("RemoveRequestFilter: %v", err)
	}
}

func TestAddEndpointIPReturnsID(t *testing.T) {
	client := newTestClient(t, `{"data":{"id":"ip-9","ip":"198.51.100.4"}}`)

	entry, err := client.AddEndpointIP(context.Background(), "1", "198.51.100.4")
	if err != nil {
		t.Fatalf("AddEndpointIP: %v", err)
	}
	if entry.ID != "ip-9" || entry.Value != "198.51.100.4" {
		t.Errorf("entry = %+v", entry)
	}
}

func TestAddEndpointIPRejectsEnvelopeError(t *testing.T) {
	client := newTestClient(t, `{"error":"that address is already allowed"}`)

	if _, err := client.AddEndpointIP(context.Background(), "1", "198.51.100.4"); err == nil {
		t.Fatal("an error inside a 200 body has to surface as an error")
	}
}

// TestEmptyWritesAreSkipped covers the configuration that manages none of the
// toggles or buckets. An empty body is a pointless call at best, so the client
// makes none at all.
func TestEmptyWritesAreSkipped(t *testing.T) {
	var calls int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	t.Cleanup(server.Close)

	quicknode, err := New("test-key", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	if err := quicknode.SetSecurityOptions(ctx, "1", SecurityOptionsPatch{}); err != nil {
		t.Fatalf("SetSecurityOptions: %v", err)
	}
	if err := quicknode.SetRateLimits(ctx, "1", RateLimitOverrides{}); err != nil {
		t.Fatalf("SetRateLimits: %v", err)
	}
	if calls != 0 {
		t.Errorf("made %d requests for writes that manage nothing, want 0", calls)
	}
}
