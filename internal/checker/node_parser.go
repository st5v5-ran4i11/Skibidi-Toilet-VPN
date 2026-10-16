package checker

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/model"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/progress"
)

// ParseNode разбирает сырую строку прокси-конфигурации и возвращает общую структуру model.NodeInfo
func ParseNode(raw string) (model.NodeInfo, error) {
	raw = strings.TrimSpace(raw)
	node := model.NodeInfo{
		Raw: raw,
	}

	switch {
	case strings.HasPrefix(raw, "vless://"):
		return parseVLESS(node)
	case strings.HasPrefix(raw, "trojan://"):
		return parseTrojan(node)
	case strings.HasPrefix(raw, "ss://"):
		return parseSS(node)
	case strings.HasPrefix(raw, "vmess://"):
		return parseVMess(node)
	case strings.HasPrefix(raw, "hysteria://"):
		return parseHysteria(node)
	case strings.HasPrefix(raw, "hysteria2://"):
		return parseHysteria2(node)
	case strings.HasPrefix(raw, "tuic://"):
		return parseTUIC(node)
	case strings.HasPrefix(raw, "http://"), strings.HasPrefix(raw, "https://"):
		return parseHTTPProxy(node)
	case strings.HasPrefix(raw, "socks://"), strings.HasPrefix(raw, "socks5://"), strings.HasPrefix(raw, "socks4://"), strings.HasPrefix(raw, "socks4a://"):
		return parseSOCKSProxy(node)
	default:
		return node, errors.New("unsupported protocol")
	}
}

func parseVLESS(node model.NodeInfo) (model.NodeInfo, error) {
	u, err := url.Parse(node.Raw)
	if err != nil {
		return node, err
	}
	node.Protocol = "vless"
	node.UUID = u.User.Username()
	node.Host = u.Hostname()
	node.Port = u.Port()

	q := u.Query()
	node.Security = q.Get("security")
	node.SNI = q.Get("sni")
	node.Fingerprint = q.Get("fp")
	node.PublicKey = q.Get("pbk")
	node.ShortID = q.Get("sid")
	node.Flow = q.Get("flow")
	node.Network = q.Get("type")
	node.Path = q.Get("path")
	node.HostHeader = q.Get("host")
	return node, nil
}

func parseTrojan(node model.NodeInfo) (model.NodeInfo, error) {
	u, err := url.Parse(node.Raw)
	if err != nil {
		return node, err
	}
	node.Protocol = "trojan"
	node.Password = u.User.Username()
	node.Host = u.Hostname()
	node.Port = u.Port()

	q := u.Query()
	node.SNI = q.Get("sni")
	node.Security = q.Get("security")
	node.Fingerprint = q.Get("fp")
	node.Network = q.Get("type")
	node.Path = q.Get("path")
	node.HostHeader = q.Get("host")
	return node, nil
}

func parseTUIC(node model.NodeInfo) (model.NodeInfo, error) {
	u, err := url.Parse(node.Raw)
	if err != nil {
		return node, err
	}
	node.Protocol = "tuic"
	node.Host = u.Hostname()
	node.Port = u.Port()
	node.UUID = u.User.Username()
	if password, ok := u.User.Password(); ok {
		node.Password = password
	}
	q := u.Query()
	node.Security = q.Get("security")
	node.SNI = q.Get("sni")
	node.Network = "udp"
	return node, nil
}

func parseSS(node model.NodeInfo) (model.NodeInfo, error) {
	u, err := url.Parse(node.Raw)
	if err != nil {
		return node, err
	}
	node.Protocol = "shadowsocks"
	node.Host = u.Hostname()
	node.Port = u.Port()

	user := u.User.String()
	decoded, err := base64.RawStdEncoding.DecodeString(user)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(user)
	}
	if err == nil {
		data := strings.SplitN(string(decoded), ":", 2)
		if len(data) == 2 {
			node.Method = data[0]
			node.Password = data[1]
		}
	}
	return node, nil
}

func parseVMess(node model.NodeInfo) (model.NodeInfo, error) {
	node.Protocol = "vmess"
	data := strings.TrimPrefix(node.Raw, "vmess://")
	data = strings.TrimSpace(data)

	decoded, err := base64.RawStdEncoding.DecodeString(data)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(data)
	}
	if err != nil {
		return node, err
	}

	var config map[string]any
	if err = json.Unmarshal(decoded, &config); err != nil {
		return node, err
	}

	if v, ok := config["id"].(string); ok {
		node.UUID = v
	}
	if v, ok := config["add"].(string); ok {
		node.Host = v
	}
	switch v := config["port"].(type) {
	case string:
		node.Port = v
	case float64:
		node.Port = strconv.Itoa(int(v))
	}
	if v, ok := config["tls"].(string); ok {
		node.Security = v
	}
	if v, ok := config["sni"].(string); ok {
		node.SNI = v
	}
	if v, ok := config["net"].(string); ok {
		node.Network = v
	}
	if v, ok := config["path"].(string); ok {
		node.Path = v
	}
	if v, ok := config["host"].(string); ok {
		node.HostHeader = v
	}
	return node, nil
}

func parseHysteria(node model.NodeInfo) (model.NodeInfo, error) {
	u, err := url.Parse(node.Raw)
	if err != nil {
		return node, err
	}
	node.Protocol = "hysteria"
	node.Host = u.Hostname()
	node.Port = u.Port()
	if u.User != nil {
		node.Password = u.User.Username()
		if password, ok := u.User.Password(); ok {
			if node.Password == "" {
				node.Password = password
			}
		}
	}
	q := u.Query()
	node.SNI = q.Get("sni")
	node.Security = q.Get("insecure")
	node.Network = q.Get("protocol")
	node.Path = q.Get("obfs")
	if node.HostHeader == "" {
		node.HostHeader = q.Get("host")
	}
	return node, nil
}

func parseHysteria2(node model.NodeInfo) (model.NodeInfo, error) {
	u, err := url.Parse(node.Raw)
	if err != nil {
		return node, err
	}
	node.Protocol = "hysteria2"
	node.Host = u.Hostname()
	node.Port = u.Port()
	if u.User != nil {
		node.Password = u.User.Username()
		if password, ok := u.User.Password(); ok {
			if node.Password == "" {
				node.Password = password
			}
		}
	}
	q := u.Query()
	node.SNI = q.Get("sni")
	node.Security = q.Get("insecure")
	node.Network = q.Get("obfs")
	node.Path = q.Get("obfs-password")
	if node.HostHeader == "" {
		node.HostHeader = q.Get("host")
	}
	return node, nil
}

func parseHTTPProxy(node model.NodeInfo) (model.NodeInfo, error) {
	u, err := url.Parse(node.Raw)
	if err != nil {
		return node, err
	}
	node.Protocol = "http"
	node.Host = u.Hostname()
	node.Port = u.Port()
	node.Username = ""
	node.Password = ""
	if u.User != nil {
		node.Username = u.User.Username()
		if p, ok := u.User.Password(); ok {
			node.Password = p
		}
	}
	node.Security = ""
	if strings.EqualFold(u.Scheme, "https") {
		node.Security = "tls"
	}
	node.SNI = u.Hostname()
	return node, nil
}
func parseSOCKSProxy(node model.NodeInfo) (model.NodeInfo, error) {
	u, err := url.Parse(node.Raw)
	if err != nil {
		return node, err
	}
	node.Protocol = "socks"
	node.Host = u.Hostname()
	node.Port = u.Port()
	if u.User != nil {
		node.Username = u.User.Username()
		if p, ok := u.User.Password(); ok {
			node.Password = p
		}
	}
	switch strings.ToLower(u.Scheme) {
	case "socks4", "socks4a":
		node.Network = "4"
	default:
		node.Network = "5"
	}
	return node, nil
}

func ParseNodes(lines []string, pm *progress.Manager) []model.NodeInfo {
	nodes := make([]model.NodeInfo, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		node, err := ParseNode(line)
		if err != nil || node.Protocol == "" || node.Host == "" || node.Port == "" {
			if pm != nil {
				pm.Fail("PARSER")
			}
			continue
		}
		nodes = append(nodes, node)
		if pm != nil {
			pm.Success("PARSER")
		}
	}
	return nodes
}

// Вставьте этот код в тот же файл node_parser.go сразу под Блоком 1

func BuildSingBoxOutbound(node model.NodeInfo, tag string) (map[string]any, error) {
	var (
		out map[string]any
		err error
	)

	switch node.Protocol {
	case "vless":
		out, err = buildVLESS(node)
	case "vmess":
		out, err = buildVMess(node)
	case "trojan":
		out, err = buildTrojan(node)
	case "shadowsocks":
		out, err = buildShadowsocks(node)
	case "hysteria":
		out, err = buildHysteria(node)
	case "hysteria2":
		out, err = buildHysteria2(node)
	case "tuic":
		out, err = buildTUIC(node)
	case "http":
		out, err = buildHTTPProxy(node)
	case "socks":
		out, err = buildSOCKSProxy(node)
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", node.Protocol)
	}

	if err != nil {
		return nil, err
	}

	out["tag"] = tag
	return out, nil
}

func buildVLESS(node model.NodeInfo) (map[string]any, error) {
	if node.UUID == "" {
		return nil, fmt.Errorf("vless uuid missing")
	}
	out := map[string]any{
		"type":        "vless",
		"server":      node.Host,
		"server_port": parsePort(node.Port),
		"uuid":        node.UUID,
	}
	if node.Flow != "" {
		out["flow"] = node.Flow
	}
	if node.Security == "tls" || node.Security == "reality" || node.SNI != "" {
		out["tls"] = buildTLS(node)
	}
	switch node.Network {
	case "ws":
		transport := map[string]any{"type": "ws"}
		if node.Path != "" {
			transport["path"] = node.Path
		}
		if node.HostHeader != "" {
			transport["headers"] = map[string]any{"Host": node.HostHeader}
		}
		out["transport"] = transport
	case "grpc":
		out["transport"] = map[string]any{"type": "grpc", "service_name": node.Path}
	case "", "tcp", "none":
		// Default TCP transport.
	default:
		return nil, fmt.Errorf("unsupported vless transport: %s", node.Network)
	}
	return out, nil
}

func buildVMess(node model.NodeInfo) (map[string]any, error) {
	if node.UUID == "" {
		return nil, fmt.Errorf("vmess uuid missing")
	}
	out := map[string]any{
		"type":        "vmess",
		"server":      node.Host,
		"server_port": parsePort(node.Port),
		"uuid":        node.UUID,
	}
	switch node.Network {
	case "ws":
		transport := map[string]any{"type": "ws"}
		if node.Path != "" {
			transport["path"] = node.Path
		}
		if node.HostHeader != "" {
			transport["headers"] = map[string]any{"Host": node.HostHeader}
		}
		out["transport"] = transport
	case "grpc":
		out["transport"] = map[string]any{"type": "grpc", "service_name": node.Path}
	case "", "tcp", "none":
		// Default TCP transport.
	default:
		return nil, fmt.Errorf("unsupported vmess transport: %s", node.Network)
	}
	if node.SNI != "" || node.Security == "tls" {
		out["tls"] = buildTLS(node)
	}
	return out, nil
}

func buildTrojan(node model.NodeInfo) (map[string]any, error) {
	if node.Password == "" {
		return nil, fmt.Errorf("trojan password missing")
	}
	out := map[string]any{
		"type":        "trojan",
		"server":      node.Host,
		"server_port": parsePort(node.Port),
		"password":    node.Password,
		"tls":         buildTLS(node),
	}
	switch node.Network {
	case "ws":
		transport := map[string]any{"type": "ws"}
		if node.Path != "" {
			transport["path"] = node.Path
		}
		if node.HostHeader != "" {
			transport["headers"] = map[string]any{"Host": node.HostHeader}
		}
		out["transport"] = transport
	case "grpc":
		out["transport"] = map[string]any{"type": "grpc", "service_name": node.Path}
	case "", "tcp", "none":
		// Default TCP transport.
	default:
		return nil, fmt.Errorf("unsupported trojan transport: %s", node.Network)
	}
	return out, nil
}

func buildShadowsocks(node model.NodeInfo) (map[string]any, error) {
	if node.Method == "" || node.Password == "" {
		return nil, fmt.Errorf("shadowsocks credentials missing")
	}
	return map[string]any{
		"type":        "shadowsocks",
		"server":      node.Host,
		"server_port": parsePort(node.Port),
		"method":      node.Method,
		"password":    node.Password,
	}, nil
}

func buildHysteria(node model.NodeInfo) (map[string]any, error) {
	if node.Host == "" {
		return nil, fmt.Errorf("hysteria host missing")
	}
	out := map[string]any{
		"type":        "hysteria",
		"server":      node.Host,
		"server_port": parsePort(node.Port),
		"tls":         buildTLS(node),
	}
	if node.Password != "" {
		out["password"] = node.Password
	}
	if node.Network != "" {
		out["protocol"] = node.Network
	}
	if node.Path != "" {
		out["obfs"] = map[string]any{"type": node.Path}
	}
	return out, nil
}

func buildHysteria2(node model.NodeInfo) (map[string]any, error) {
	if node.Password == "" {
		return nil, fmt.Errorf("hysteria2 password missing")
	}
	out := map[string]any{
		"type":        "hysteria2",
		"server":      node.Host,
		"server_port": parsePort(node.Port),
		"password":    node.Password,
		"tls":         buildTLS(node),
	}
	if node.Network != "" {
		obfs := map[string]any{"type": node.Network}
		if node.Path != "" {
			obfs["password"] = node.Path
		}
		out["obfs"] = obfs
	}
	return out, nil
}

func buildTUIC(node model.NodeInfo) (map[string]any, error) {
	if node.UUID == "" {
		return nil, fmt.Errorf("tuic uuid missing")
	}
	if node.Password == "" {
		return nil, fmt.Errorf("tuic password missing")
	}
	return map[string]any{
		"type":               "tuic",
		"server":             node.Host,
		"server_port":        parsePort(node.Port),
		"uuid":               node.UUID,
		"password":           node.Password,
		"congestion_control": "bbr",
		"udp_relay_mode":     "native",
		"tls":                buildTLS(node),
	}, nil
}

func buildTLS(node model.NodeInfo) map[string]any {
	tls := map[string]any{"enabled": true}
	if node.SNI != "" {
		tls["server_name"] = node.SNI
	}
	if node.Fingerprint != "" {
		tls["utls"] = map[string]any{"enabled": true, "fingerprint": node.Fingerprint}
	}
	if node.Security == "reality" && node.PublicKey != "" {
		tls["reality"] = map[string]any{"enabled": true, "public_key": node.PublicKey, "short_id": node.ShortID}
	}
	return tls
}

func parsePort(port string) int {
	p, err := strconv.Atoi(port)
	if err != nil {
		return 0
	}
	return p
}

func buildHTTPProxy(node model.NodeInfo) (map[string]any, error) {
	out := map[string]any{"type": "http", "server": node.Host, "server_port": parsePort(node.Port)}
	if node.Username != "" {
		out["username"] = node.Username
	}
	if node.Password != "" {
		out["password"] = node.Password
	}
	if node.Security == "tls" {
		out["tls"] = map[string]any{"enabled": true, "server_name": node.SNI}
	}
	return out, nil
}
func buildSOCKSProxy(node model.NodeInfo) (map[string]any, error) {
	version := "5"
	if node.Network == "4" {
		version = "4"
	}
	out := map[string]any{"type": "socks", "server": node.Host, "server_port": parsePort(node.Port), "version": version}
	if node.Username != "" {
		out["username"] = node.Username
	}
	if node.Password != "" {
		out["password"] = node.Password
	}
	return out, nil
}
