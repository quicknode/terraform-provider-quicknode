package client

import (
	"strings"
	"testing"
)

func TestRedactEndpointURL(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "token followed by a chain suffix",
			raw:  "https://example-name.hype-testnet.quiknode.pro/abc123/evm",
			want: "https://example-name.hype-testnet.quiknode.pro/REPLACE_WITH_TOKEN/evm",
		},
		{
			name: "token with no suffix",
			raw:  "https://example-name.quiknode.pro/abc123",
			want: "https://example-name.quiknode.pro/REPLACE_WITH_TOKEN",
		},
		{
			name: "trailing slash after the token",
			raw:  "https://example-name.btc.quiknode.pro/abc123/",
			want: "https://example-name.btc.quiknode.pro/REPLACE_WITH_TOKEN/",
		},
		{
			name: "websocket scheme",
			raw:  "wss://example-name.quiknode.pro/abc123/evm",
			want: "wss://example-name.quiknode.pro/REPLACE_WITH_TOKEN/evm",
		},
		{
			name: "no path at all",
			raw:  "https://example-name.quiknode.pro",
			want: "https://example-name.quiknode.pro",
		},
		{
			name: "empty",
			raw:  "",
			want: "",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := RedactEndpointURL(testCase.raw); got != testCase.want {
				t.Errorf("RedactEndpointURL(%q) = %q, want %q", testCase.raw, got, testCase.want)
			}
		})
	}
}

// TestRedactedURLKeepsItsShape checks what the placeholder exists for:
// substituting a token has to reproduce the original URL exactly, including a
// suffix the chain appends after the token.
func TestRedactedURLKeepsItsShape(t *testing.T) {
	for _, raw := range []string{
		"https://example-name.hype-testnet.quiknode.pro/abc123/evm",
		"https://example-name.btc.quiknode.pro/abc123/",
		"wss://example-name.quiknode.pro/abc123",
	} {
		redacted := RedactEndpointURL(raw)
		if redacted == raw {
			t.Errorf("RedactEndpointURL(%q) left the credential in place", raw)
			continue
		}
		restored := strings.Replace(redacted, URLTokenPlaceholder, "abc123", 1)
		if restored != raw {
			t.Errorf("substituting the token gave %q, want %q", restored, raw)
		}
	}
}
