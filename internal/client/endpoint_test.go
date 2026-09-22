package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// hypeEndpointBody mirrors a live GET /v0/endpoints/{id} response for a chain
// that appends a path suffix after the token. The token is fabricated.
const hypeEndpointBody = `{"data":{"id":"652052","label":null,"chain":"hype","network":"hype-testnet",
"http_url":"https://polished-damp-grass.hype-testnet.quiknode.pro/TOKENVALUE/evm",
"wss_url":"wss://polished-damp-grass.hype-testnet.quiknode.pro/TOKENVALUE/evm",
"security":{"options":{"tokens":true,"cors":true},
"tokens":[{"id":"d3312bd2-c1a2-4d89-865f-11c99fa3863a","token":"TOKENVALUE"}],
"jwts":null,"referrers":null,"domain_masks":null,"ips":null,"request_filters":null},
"status":"active","rate_limits":{"rate_limit_by_ip":false,"account":-1,"rps":-1,"rpd":-1,"rpm":-1},
"tags":[{"tag_id":7,"label":"prod"}],"is_multichain":false}}`

// bitcoinEndpointBody mirrors a live response for a chain with no WebSocket
// support and no path suffix.
const bitcoinEndpointBody = `{"data":{"id":"613071","label":"ledger","chain":"btc","network":"btc",
"http_url":"https://frosty-capable-pallet.btc.quiknode.pro/TOKENVALUE/","wss_url":null,
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
	endpoint, err := newTestClient(t, hypeEndpointBody).GetEndpoint(context.Background(), "652052")
	if err != nil {
		t.Fatalf("GetEndpoint: %v", err)
	}

	const wantWorking = "https://polished-damp-grass.hype-testnet.quiknode.pro/TOKENVALUE/evm"
	if endpoint.HTTPURLWithToken != wantWorking {
		t.Errorf("HTTPURLWithToken = %q, want %q", endpoint.HTTPURLWithToken, wantWorking)
	}
	if endpoint.WSSURLWithToken != "wss://polished-damp-grass.hype-testnet.quiknode.pro/TOKENVALUE/evm" {
		t.Errorf("WSSURLWithToken = %q", endpoint.WSSURLWithToken)
	}

	// The token-free URL is not a working address on this chain, which is the
	// reason HTTPURLWithToken exists.
	if endpoint.HTTPURL == wantWorking {
		t.Error("HTTPURL still carries the token")
	}
	if endpoint.HTTPURL+"/TOKENVALUE" == wantWorking {
		t.Error("joining HTTPURL to the token happens to work here; the test no longer guards the bug it was written for")
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
	endpoint, err := newTestClient(t, bitcoinEndpointBody).GetEndpoint(context.Background(), "613071")
	if err != nil {
		t.Fatalf("GetEndpoint: %v", err)
	}

	if endpoint.WSSURL != "" || endpoint.WSSURLWithToken != "" {
		t.Errorf("WSSURL = %q, WSSURLWithToken = %q, want both empty", endpoint.WSSURL, endpoint.WSSURLWithToken)
	}
	if endpoint.HTTPURLWithToken != "https://frosty-capable-pallet.btc.quiknode.pro/TOKENVALUE/" {
		t.Errorf("HTTPURLWithToken = %q", endpoint.HTTPURLWithToken)
	}
	if endpoint.HTTPURL != "https://frosty-capable-pallet.btc.quiknode.pro" {
		t.Errorf("HTTPURL = %q", endpoint.HTTPURL)
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
