package checker

import (
	"encoding/json"
	"fmt"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/model"
)

func MarshalServiceConfig(node model.NodeInfo, mixedPort int) ([]byte, error) {
	if mixedPort <= 0 {
		return nil, fmt.Errorf("invalid mixed port")
	}
	proxy, err := BuildSingBoxOutbound(node, "proxy")
	if err != nil {
		return nil, err
	}
	cfg := map[string]any{
		"log":       map[string]any{"level": "warn", "timestamp": true},
		"inbounds":  []any{map[string]any{"type": "mixed", "tag": "service-in", "listen": "127.0.0.1", "listen_port": mixedPort}},
		"outbounds": []any{proxy, map[string]any{"type": "direct", "tag": "direct"}},
		"route":     map[string]any{"final": "proxy", "auto_detect_interface": true},
	}
	return json.Marshal(cfg)
}

func MarshalRouterConfig(services map[string]struct {
	Port    int
	Domains []string
}) ([]byte, error) {
	outbounds := []any{map[string]any{"type": "direct", "tag": "direct"}}
	rules := []any{
		map[string]any{"action": "sniff", "timeout": "300ms"},
	}
	for name, s := range services {
		if s.Port <= 0 {
			continue
		}
		outbounds = append(outbounds, map[string]any{"type": "socks", "tag": "svc-" + name, "server": "127.0.0.1", "server_port": s.Port, "version": "5"})
		exact := []string{}
		suffix := []string{}
		for _, d := range s.Domains {
			if len(d) > 2 && d[:2] == "*." {
				suffix = append(suffix, d[2:])
			} else {
				exact = append(exact, d)
			}
		}
		rule := map[string]any{"action": "route", "outbound": "svc-" + name}
		if len(exact) > 0 {
			rule["domain"] = exact
		}
		if len(suffix) > 0 {
			rule["domain_suffix"] = suffix
		}
		if len(exact) > 0 || len(suffix) > 0 {
			rules = append(rules, rule)
		}
	}
	rules = append(rules, map[string]any{"action": "route", "outbound": "direct"})
	cfg := map[string]any{
		"log": map[string]any{"level": "warn", "timestamp": true},
		"dns": map[string]any{
			"servers": []any{
				map[string]any{"type": "https", "tag": "remote", "server": "dns.google", "server_port": 443, "path": "/dns-query", "tls": map[string]any{"server_name": "dns.google"}},
				map[string]any{"type": "https", "tag": "direct", "server": "223.5.5.5", "server_port": 443, "path": "/dns-query", "tls": map[string]any{"server_name": "223.5.5.5", "insecure": true}},
			},
			"rules": []any{map[string]any{"action": "route", "server": "remote"}},
			"final": "remote",
		},
		"inbounds": []any{map[string]any{
			"type": "tun", "tag": "router-tun", "interface_name": "SkibidiVPN", "address": []string{"172.31.255.1/30", "fd00:5b1d:1::1/126"},
			"mtu": 1500, "auto_route": true, "strict_route": true, "stack": "gvisor", "dns_mode": "hijack",
		}},
		"outbounds": outbounds,
		"route":     map[string]any{"auto_detect_interface": true, "rules": rules, "final": "direct"},
	}
	return json.Marshal(cfg)
}
