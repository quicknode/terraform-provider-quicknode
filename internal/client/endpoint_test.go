package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// hypeEndpointBody mirrors a live GET /v0/endpoints/{id} response for a chain
// that appends a path suffix after the token. The token is fabricated.
const hypeEndpointBody = `{"data":{"id":"123456","label":null,"chain":"hype","network":"hype-testnet",
"http_url":"https://example-name.hype-testnet.quiknode.pro/TOKENVALUE/evm",
"wss_url":"wss://example-name.hype-testnet.quiknode.pro/TOKENVALUE/evm",
"security":{"options":{"tokens":true,"cors":true},
"tokens":[{"id":"e5d4c3b2-a1f0-4876-9432-10fedcba9876","token":"TOKENVALUE"}],
"jwts":null,"referrers":null,"domain_masks":null,"ips":null,"request_filters":null},
"status":"active","rate_limits":{"rate_limit_by_ip":false,"account":-1,"rps":-1,"rpd":-1,"rpm":-1},
"tags":[{"tag_id":7,"label":"prod"}],"is_multichain":false}}`

// bitcoinEndpointBody mirrors a live response for a chain with no WebSocket
// support and no path suffix.
const bitcoinEndpointBody = `{"data":{"id":"123457","label":"ledger","chain":"btc","network":"btc",
"http_url":"https://example-name.btc.quiknode.pro/TOKENVALUE/","wss_url":null,
"security":{"options":{"tokens":true},"tokens":[{"id":"abc","token":"TOKENVALUE"}]},
"status":"paused","tags":[],"is_multichain":false}}`

func newTestClient(t *testing.T, body string) *Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	quicknode, err := New("test-key", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return quicknode
}

func TestGetEndpointWithPathSuffix(t *testing.T) {
	endpoint, err := newTestClient(t, hypeEndpointBody).GetEndpoint(context.Background(), "123456")
	if err != nil {
		t.Fatalf("GetEndpoint: %v", err)
	}

	const wantWorking = "https://example-name.hype-testnet.quiknode.pro/TOKENVALUE/evm"
	if endpoint.HTTPURLWithToken != wantWorking {
		t.Errorf("HTTPURLWithToken = %q, want %q", endpoint.HTTPURLWithToken, wantWorking)
	}
	if endpoint.WSSURLWithToken != "wss://example-name.hype-testnet.quiknode.pro/TOKENVALUE/evm" {
		t.Errorf("WSSURLWithToken = %q", endpoint.WSSURLWithToken)
	}
	if endpoint.SafeWSSURL != "wss://example-name.hype-testnet.quiknode.pro/REPLACE_WITH_TOKEN/evm" {
		t.Errorf("SafeWSSURL = %q", endpoint.SafeWSSURL)
	}

	// The redacted URL keeps the real one's shape, so substituting a token
	// reproduces it exactly. That is what the placeholder buys over cutting
	// the token out: this chain puts a suffix after it.
	const wantRedacted = "https://example-name.hype-testnet.quiknode.pro/REPLACE_WITH_TOKEN/evm"
	if endpoint.SafeHTTPURL != wantRedacted {
		t.Errorf("SafeHTTPURL = %q, want %q", endpoint.SafeHTTPURL, wantRedacted)
	}
	if strings.Contains(endpoint.SafeHTTPURL, "TOKENVALUE") {
		t.Error("SafeHTTPURL still carries the token")
	}

	if len(endpoint.Tokens) != 1 || endpoint.Tokens[0].Value != "TOKENVALUE" {
		t.Errorf("Tokens = %+v", endpoint.Tokens)
	}
	if len(endpoint.Tags) != 1 || endpoint.Tags[0].Label != "prod" || endpoint.Tags[0].ID != 7 {
		t.Errorf("Tags = %+v", endpoint.Tags)
	}
	if endpoint.Status != "active" || endpoint.Multichain {
		t.Errorf("Status = %q, Multichain = %v", endpoint.Status, endpoint.Multichain)
	}
}

func TestGetEndpointWithoutWebsocket(t *testing.T) {
	endpoint, err := newTestClient(t, bitcoinEndpointBody).GetEndpoint(context.Background(), "123457")
	if err != nil {
		t.Fatalf("GetEndpoint: %v", err)
	}

	if endpoint.SafeWSSURL != "" || endpoint.WSSURLWithToken != "" {
		t.Errorf("SafeWSSURL = %q, WSSURLWithToken = %q, want both empty", endpoint.SafeWSSURL, endpoint.WSSURLWithToken)
	}
	if endpoint.HTTPURLWithToken != "https://example-name.btc.quiknode.pro/TOKENVALUE/" {
		t.Errorf("HTTPURLWithToken = %q", endpoint.HTTPURLWithToken)
	}
	if endpoint.SafeHTTPURL != "https://example-name.btc.quiknode.pro/REPLACE_WITH_TOKEN/" {
		t.Errorf("SafeHTTPURL = %q", endpoint.SafeHTTPURL)
	}
	if endpoint.Status != "paused" || endpoint.Label != "ledger" {
		t.Errorf("Status = %q, Label = %q", endpoint.Status, endpoint.Label)
	}
}

func TestGetEndpointRejectsEnvelopeError(t *testing.T) {
	quicknode := newTestClient(t, `{"data":null,"error":"endpoint does not belong to this account"}`)

	if _, err := quicknode.GetEndpoint(context.Background(), "1"); err == nil {
		t.Fatal("expected an error from a 200 response carrying an error field")
	}
}

// liveEndpointBody is a verbatim GET /v0/endpoints/{id} response with the token
// replaced. It carries ipCustomHeader and responseLogging, which the published
// spec either mistypes or omits.
const liveEndpointBody = `{"data":{"id":"123456","label":null,"chain":"hype","network":"hype-testnet",` +
	`"http_url":"https://example-name.hype-testnet.quiknode.pro/TOKENVALUE/evm",` +
	`"wss_url":"wss://example-name.hype-testnet.quiknode.pro/TOKENVALUE/evm",` +
	`"security":{"options":{"tokens":true,"referrers":false,"jwts":false,"ips":false,` +
	`"domainMasks":false,"hsts":false,"cors":true,"responseLogging":true,` +
	`"requestFilters":false,"ipCustomHeader":{"value":null}},` +
	`"tokens":[{"id":"e5d4c3b2-a1f0-4876-9432-10fedcba9876","token":"TOKENVALUE"}],` +
	`"jwts":null,"referrers":null,"domain_masks":null,"ips":null,"request_filters":null},` +
	`"status":"active","rate_limits":{"rate_limit_by_ip":false,"account":-1,"rps":-1,"rpd":-1,"rpm":-1},` +
	`"tags":[],"is_multichain":false}}`

func TestGetEndpointDecodesLiveBody(t *testing.T) {
	endpoint, err := newTestClient(t, liveEndpointBody).GetEndpoint(context.Background(), "123456")
	if err != nil {
		t.Fatalf("GetEndpoint on a verbatim live body: %v", err)
	}
	if len(endpoint.Tokens) != 1 {
		t.Errorf("Tokens = %+v", endpoint.Tokens)
	}
}

// multichainURLsBody mirrors a live GET /v0/endpoints/{id}/urls response for a
// multichain endpoint, trimmed to networks that cover a path suffix and a
// missing WebSocket URL. The token is fabricated.
const multichainURLsBody = `{"data":{` +
	`"http_url":"https://example-name.ethereum-sepolia.quiknode.pro/TOKENVALUE/",` +
	`"wss_url":"wss://example-name.ethereum-sepolia.quiknode.pro/TOKENVALUE/",` +
	`"multichain_urls":{` +
	`"avalanche-mainnet":{"http_url":"https://example-name.avalanche-mainnet.quiknode.pro/TOKENVALUE/ext/bc/C/rpc/",` +
	`"wss_url":"wss://example-name.avalanche-mainnet.quiknode.pro/TOKENVALUE/ext/bc/C/ws/"},` +
	`"btc":{"http_url":"https://example-name.btc.quiknode.pro/TOKENVALUE/","wss_url":null}}},` +
	`"error":null}`

func TestGetEndpointURLsDecodesMultichain(t *testing.T) {
	urls, err := newTestClient(t, multichainURLsBody).GetEndpointURLs(context.Background(), "123456")
	if err != nil {
		t.Fatalf("GetEndpointURLs: %v", err)
	}
	if urls.SafeHTTPURL != "https://example-name.ethereum-sepolia.quiknode.pro/REPLACE_WITH_TOKEN/" {
		t.Errorf("SafeHTTPURL = %q", urls.SafeHTTPURL)
	}
	if len(urls.Multichain) != 2 {
		t.Fatalf("Multichain = %+v", urls.Multichain)
	}

	avalanche := urls.Multichain["avalanche-mainnet"]
	if avalanche.SafeHTTPURL != "https://example-name.avalanche-mainnet.quiknode.pro/REPLACE_WITH_TOKEN/ext/bc/C/rpc/" {
		t.Errorf("avalanche SafeHTTPURL = %q", avalanche.SafeHTTPURL)
	}
	if avalanche.WSSURLWithToken != "wss://example-name.avalanche-mainnet.quiknode.pro/TOKENVALUE/ext/bc/C/ws/" {
		t.Errorf("avalanche WSSURLWithToken = %q", avalanche.WSSURLWithToken)
	}

	bitcoin := urls.Multichain["btc"]
	if bitcoin.SafeWSSURL != "" || bitcoin.WSSURLWithToken != "" {
		t.Errorf("btc WebSocket URLs = %q, %q, want empty", bitcoin.SafeWSSURL, bitcoin.WSSURLWithToken)
	}
}

func TestGetEndpointURLsWithoutMultichain(t *testing.T) {
	body := `{"data":{"http_url":"https://example-name.btc.quiknode.pro/TOKENVALUE/","wss_url":null},"error":null}`
	urls, err := newTestClient(t, body).GetEndpointURLs(context.Background(), "123457")
	if err != nil {
		t.Fatalf("GetEndpointURLs: %v", err)
	}
	if urls.Multichain == nil || len(urls.Multichain) != 0 {
		t.Errorf("Multichain = %#v, want an empty map", urls.Multichain)
	}
}
