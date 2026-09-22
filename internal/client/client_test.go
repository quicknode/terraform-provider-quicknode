package client

import "testing"

func TestSplitEndpointURL(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		wantBase  string
		wantToken string
	}{
		{
			name:      "token followed by a chain suffix",
			raw:       "https://polished-damp-grass.hype-testnet.quiknode.pro/abc123/evm",
			wantBase:  "https://polished-damp-grass.hype-testnet.quiknode.pro/evm",
			wantToken: "abc123",
		},
		{
			name:      "token with no suffix",
			raw:       "https://example-name.quiknode.pro/abc123",
			wantBase:  "https://example-name.quiknode.pro",
			wantToken: "abc123",
		},
		{
			name:      "websocket scheme",
			raw:       "wss://example-name.quiknode.pro/abc123/evm",
			wantBase:  "wss://example-name.quiknode.pro/evm",
			wantToken: "abc123",
		},
		{
			name:      "no path at all",
			raw:       "https://example-name.quiknode.pro",
			wantBase:  "https://example-name.quiknode.pro",
			wantToken: "",
		},
		{
			name:      "empty",
			raw:       "",
			wantBase:  "",
			wantToken: "",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			base, token := splitEndpointURL(testCase.raw)
			if base != testCase.wantBase {
				t.Errorf("base = %q, want %q", base, testCase.wantBase)
			}
			if token != testCase.wantToken {
				t.Errorf("token = %q, want %q", token, testCase.wantToken)
			}
		})
	}
}
