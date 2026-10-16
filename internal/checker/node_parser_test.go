package checker

import (
	"strings"
	"testing"

	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/model"
)

func TestBuildSingBoxOutboundRejectsUnsupportedTransport(t *testing.T) {
	node := model.NodeInfo{Protocol: "vless", UUID: "00000000-0000-0000-0000-000000000001", Host: "example.com", Port: "443", Network: "xhttp"}
	_, err := BuildSingBoxOutbound(node, "test")
	if err == nil || !strings.Contains(err.Error(), "unsupported vless transport") {
		t.Fatalf("expected explicit unsupported transport error, got %v", err)
	}
}

func TestParseHTTPAndSOCKS(t *testing.T) {
	cases := []struct{ raw, proto string }{
		{"http://127.0.0.1:8080", "http"},
		{"https://user:pass@example.com:443", "http"},
		{"socks5://user:pass@example.com:1080", "socks"},
		{"socks4://user@example.com:1080", "socks"},
	}
	for _, tc := range cases {
		n, err := ParseNode(tc.raw)
		if err != nil {
			t.Fatalf("%s: %v", tc.raw, err)
		}
		if n.Protocol != tc.proto || n.Host == "" || n.Port == "" {
			t.Fatalf("bad parse: %#v", n)
		}
	}
}
