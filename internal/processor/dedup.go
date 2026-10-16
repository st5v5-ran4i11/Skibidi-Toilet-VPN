package processor

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

type ConcurrentDedup struct {
	mu   sync.RWMutex
	seen map[string]bool
}

func NewConcurrentDedup() *ConcurrentDedup {
	return &ConcurrentDedup{seen: make(map[string]bool)}
}

// Fingerprint даёт один идентификатор на сетевую точку HOST:PORT.
// Дополнительно декодируется VMess и старый SS-формат, у которых endpoint
// может быть скрыт внутри base64. Это устраняет дубли между разными протоколами
// и разными форматами одной и той же подписки.
func (d *ConcurrentDedup) Fingerprint(line string) string {
	hostPort := extractHostPort(line)
	if hostPort == "" {
		return ""
	}

	hash := sha256.Sum256([]byte(hostPort))
	return hex.EncodeToString(hash[:])
}

func extractHostPort(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}

	if unescaped, err := url.PathUnescape(line); err == nil {
		line = unescaped
	}

	lower := strings.ToLower(line)
	if strings.HasPrefix(lower, "vmess://") {
		if host, port := decodeVMessEndpoint(line); host != "" && port != "" {
			return normalizeHostPort(host, port)
		}
	}

	// Некоторые SS-подписки имеют вид ss://base64(method:password@host:port).
	if strings.HasPrefix(lower, "ss://") {
		if host, port := decodeSSEndpoint(line); host != "" && port != "" {
			return normalizeHostPort(host, port)
		}
	}

	idxProto := strings.Index(line, "://")
	if idxProto >= 0 {
		line = line[idxProto+3:]
	}

	if idxHash := strings.Index(line, "#"); idxHash >= 0 {
		line = line[:idxHash]
	}
	if idxQuery := strings.Index(line, "?"); idxQuery >= 0 {
		line = line[:idxQuery]
	}

	if idxAt := strings.LastIndex(line, "@"); idxAt >= 0 {
		line = line[idxAt+1:]
	}

	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return ""
	}

	if host, port, err := net.SplitHostPort(line); err == nil {
		return normalizeHostPort(host, port)
	}

	// Пробуем URL-парсер как fallback для экзотических вариантов IPv6/URI.
	if u, err := url.Parse("tcp://" + line); err == nil && u.Hostname() != "" && u.Port() != "" {
		return normalizeHostPort(u.Hostname(), u.Port())
	}

	return ""
}

func decodeVMessEndpoint(line string) (string, string) {
	payload := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "vmess://"), "VMESS://"))
	data, ok := decodeBase64Payload(payload)
	if !ok {
		return "", ""
	}

	var cfg map[string]any
	if json.Unmarshal(data, &cfg) != nil {
		return "", ""
	}

	host, _ := cfg["add"].(string)
	var port string
	switch value := cfg["port"].(type) {
	case string:
		port = value
	case float64:
		port = strconv.Itoa(int(value))
	case int:
		port = strconv.Itoa(value)
	}
	return host, port
}

func decodeSSEndpoint(line string) (string, string) {
	payload := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "ss://"), "SS://"))
	if idx := strings.Index(payload, "#"); idx >= 0 {
		payload = payload[:idx]
	}
	if idx := strings.Index(payload, "?"); idx >= 0 {
		payload = payload[:idx]
	}

	// Новая форма уже содержит @host:port.
	if idx := strings.LastIndex(payload, "@"); idx >= 0 {
		endpoint := payload[idx+1:]
		if host, port, err := net.SplitHostPort(endpoint); err == nil {
			return host, port
		}
	}

	data, ok := decodeBase64Payload(payload)
	if !ok {
		return "", ""
	}
	decoded := string(data)
	if idx := strings.LastIndex(decoded, "@"); idx >= 0 {
		endpoint := decoded[idx+1:]
		if host, port, err := net.SplitHostPort(endpoint); err == nil {
			return host, port
		}
	}
	return "", ""
}

func decodeBase64Payload(payload string) ([]byte, bool) {
	payload = strings.TrimSpace(payload)
	payload = strings.TrimRight(payload, "=")
	if payload == "" {
		return nil, false
	}

	candidates := []string{payload, strings.NewReplacer("-", "+", "_", "/").Replace(payload)}
	for _, candidate := range candidates {
		padded := candidate
		switch len(padded) % 4 {
		case 2:
			padded += "=="
		case 3:
			padded += "="
		}
		for _, decoder := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding} {
			if data, err := decoder.DecodeString(padded); err == nil && len(data) > 0 {
				return data, true
			}
		}
	}
	return nil, false
}

func normalizeHostPort(host, port string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	port = strings.TrimSpace(port)
	if host == "" || port == "" {
		return ""
	}
	if p, err := strconv.Atoi(port); err != nil || p < 1 || p > 65535 {
		return ""
	}
	return net.JoinHostPort(host, port)
}

func (d *ConcurrentDedup) IsUnique(line string) bool {
	fp := d.Fingerprint(line)
	if fp == "" {
		return false
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.seen[fp] {
		return false
	}
	d.seen[fp] = true
	return true
}

func (d *ConcurrentDedup) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seen = make(map[string]bool)
}
