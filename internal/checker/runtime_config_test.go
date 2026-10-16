package checker

import (
	"encoding/json"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/model"
	"testing"
)

func TestMarshalServiceConfig(t *testing.T) {
	data, err := MarshalServiceConfig(model.NodeInfo{Protocol: "http", Host: "127.0.0.1", Port: "8080"}, 19080)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg["inbounds"].([]any)) != 1 {
		t.Fatal("expected mixed inbound")
	}
}
func TestMarshalRouterConfig(t *testing.T) {
	data, err := MarshalRouterConfig(map[string]struct {
		Port    int
		Domains []string
	}{
		"youtube": {Port: 21080, Domains: []string{"youtube.com", "*.youtube.com"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg["outbounds"].([]any)) != 2 {
		t.Fatal("expected direct + service outbound")
	}
}
