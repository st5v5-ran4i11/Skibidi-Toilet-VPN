package ipc

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/config"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/core/engine"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/model"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/services"
)

func TestLoopbackIPCAuthenticationAndSnapshot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "configs"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "configs", "sources.yaml"), []byte("sources: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	eng := engine.New(cfg, root)
	srv := New(root, cfg, eng)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Close(ctx)
	}()

	endpointBytes, err := os.ReadFile(filepath.Join(root, "data", "ipc.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ep Endpoint
	if err := json.Unmarshal(endpointBytes, &ep); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + ep.Address + "/api/v1/snapshot")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d, want 401", resp.StatusCode)
	}
	req, err := http.NewRequest(http.MethodGet, "http://"+ep.Address+"/api/v1/snapshot", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+ep.Token)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("authenticated status=%d, want 200", resp.StatusCode)
	}
	var snap Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatal(err)
	}
	if snap.Version != ProtocolVersion {
		t.Fatalf("version=%d, want %d", snap.Version, ProtocolVersion)
	}
	if snap.Pipeline != "IDLE" {
		t.Fatalf("pipeline=%q, want IDLE", snap.Pipeline)
	}
	if !snap.Capabilities.ServiceManager {
		t.Fatal("service manager snapshot API should be advertised")
	}
}

func TestSafePoolSnapshotDoesNotExposeCredentials(t *testing.T) {
	pool := map[string]services.Pool{
		"youtube": {
			Service: "youtube",
			Primary: &services.PoolEntry{Node: model.NodeInfo{ID: "node-123", DisplayName: "YT_Test", UUID: "secret-uuid", Password: "secret-password"}, OriginalName: "Test", State: services.Active},
		},
	}
	encoded, err := json.Marshal(safePools(pool))
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if strings.Contains(text, "secret-password") || strings.Contains(text, "secret-uuid") {
		t.Fatalf("safe pool snapshot leaked credentials: %s", text)
	}
	if !strings.Contains(text, "node-123") || !strings.Contains(text, "YT_Test") {
		t.Fatalf("safe pool snapshot omitted safe identity/display fields: %s", text)
	}
}

func TestLogsEndpointReturnsBoundedTail(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "configs"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "configs", "sources.yaml"), []byte("sources: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "logs", "application.log"), []byte("line-1\nline-2\nline-3\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	srv := New(root, cfg, engine.New(cfg, root))
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Close(ctx)
	}()
	endpointBytes, err := os.ReadFile(filepath.Join(root, "data", "ipc.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ep Endpoint
	if err := json.Unmarshal(endpointBytes, &ep); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodGet, "http://"+ep.Address+"/api/v1/logs?limit=2", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+ep.Token)
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var payload struct {
		Version int      `json:"version"`
		Lines   []string `json:"lines"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Lines) != 2 || payload.Lines[0] != "line-2" || payload.Lines[1] != "line-3" {
		t.Fatalf("unexpected log tail: %#v", payload.Lines)
	}
}
