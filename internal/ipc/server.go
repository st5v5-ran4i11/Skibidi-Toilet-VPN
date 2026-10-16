package ipc

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/config"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/core/engine"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/core/events"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/progress"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/services"
	"gopkg.in/yaml.v3"
)

const ProtocolVersion = 1
const maxRequestBytes = 1 << 20

type Endpoint struct {
	Version int    `json:"version"`
	Address string `json:"address"`
	Token   string `json:"token"`
	PID     int    `json:"pid"`
}
type Snapshot struct {
	Version      int                               `json:"version"`
	Timestamp    time.Time                         `json:"timestamp"`
	Backend      string                            `json:"backend"`
	Pipeline     string                            `json:"pipeline"`
	Stages       []progress.Stage                  `json:"stages"`
	Sources      []SourceStatus                    `json:"sources"`
	Services     []ServiceStatus                   `json:"services"`
	Pools        map[string]NodePoolStatus         `json:"pools"`
	Runtimes     map[string]services.RuntimeStatus `json:"runtimes"`
	Router       string                            `json:"router"`
	Capabilities Capabilities                      `json:"capabilities"`
}
type PoolEntryStatus struct {
	NodeID       string    `json:"node_id"`
	DisplayName  string    `json:"display_name"`
	OriginalName string    `json:"original_name,omitempty"`
	Score        float64   `json:"score"`
	State        string    `json:"state"`
	Failures     int       `json:"failures"`
	LastSuccess  time.Time `json:"last_success,omitempty"`
}
type NodePoolStatus struct {
	Service        string            `json:"service"`
	Primary        *PoolEntryStatus  `json:"primary,omitempty"`
	Secondary      *PoolEntryStatus  `json:"secondary,omitempty"`
	Standby        []PoolEntryStatus `json:"standby,omitempty"`
	CandidateCount int               `json:"candidate_count"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

type Capabilities struct {
	PipelineControl bool `json:"pipeline_control"`
	SourceListing   bool `json:"source_listing"`
	SettingsRead    bool `json:"settings_read"`
	SettingsWrite   bool `json:"settings_write"`
	ServiceManager  bool `json:"service_manager"`
	RuntimeStatus   bool `json:"runtime_status"`
}
type SourceStatus struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
}
type ServiceStatus struct {
	ID              string     `json:"id"`
	State           string     `json:"state"`
	Available       bool       `json:"available"`
	PoolSize        int        `json:"pool_size"`
	PoolNodes       []string   `json:"pool_nodes,omitempty"`
	Primary         string     `json:"primary,omitempty"`
	Secondary       string     `json:"secondary,omitempty"`
	Standby         int        `json:"standby"`
	RuntimeState    string     `json:"runtime_state"`
	PID             int        `json:"pid,omitempty"`
	Port            int        `json:"port,omitempty"`
	LatencyMs       int64      `json:"latency_ms,omitempty"`
	LastHealthCheck *time.Time `json:"last_health_check,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
	Message         string     `json:"message,omitempty"`
}
type Command struct {
	Version     int    `json:"version"`
	ID          string `json:"id"`
	Action      string `json:"action"`
	ServiceID   string `json:"service_id,omitempty"`
	NetworkMode string `json:"network_mode,omitempty"`
}
type CommandResult struct {
	Version  int       `json:"version"`
	ID       string    `json:"id"`
	Accepted bool      `json:"accepted"`
	Status   string    `json:"status"`
	Error    string    `json:"error,omitempty"`
	Snapshot *Snapshot `json:"snapshot,omitempty"`
}
type LogEvent struct {
	Version   int       `json:"version"`
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Component string    `json:"component"`
	Message   string    `json:"message"`
}

type Server struct {
	root         string
	cfg          *config.Config
	engine       *engine.Engine
	server       *http.Server
	endpoint     Endpoint
	endpointPath string
	once         sync.Once
}

func New(root string, cfg *config.Config, eng *engine.Engine) *Server {
	return &Server{root: root, cfg: cfg, engine: eng, endpointPath: filepath.Join(root, "data", "ipc.json")}
}
func (s *Server) Endpoint() Endpoint { return s.endpoint }
func (s *Server) Start() error {
	if err := os.MkdirAll(filepath.Dir(s.endpointPath), 0700); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		listener.Close()
		return err
	}
	s.endpoint = Endpoint{Version: ProtocolVersion, Address: listener.Addr().String(), Token: hex.EncodeToString(secret), PID: os.Getpid()}
	data, err := json.MarshalIndent(s.endpoint, "", "  ")
	if err != nil {
		listener.Close()
		return err
	}
	if err = os.WriteFile(s.endpointPath, data, 0600); err != nil {
		listener.Close()
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", s.authorize(s.handleHealth))
	mux.HandleFunc("GET /api/v1/snapshot", s.authorize(s.handleSnapshot))
	mux.HandleFunc("GET /api/v1/sources", s.authorize(s.handleSources))
	mux.HandleFunc("GET /api/v1/logs", s.authorize(s.handleLogs))
	mux.HandleFunc("GET /api/v1/settings", s.authorize(s.handleSettings))
	mux.HandleFunc("PUT /api/v1/settings/network-mode", s.authorize(s.handleSetNetworkMode))
	mux.HandleFunc("POST /api/v1/commands", s.authorize(s.handleCommand))
	mux.HandleFunc("GET /api/v1/events", s.authorize(s.handleEvents))
	s.server = &http.Server{Handler: securityHeaders(mux), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 0, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	go func() { _ = s.server.Serve(listener) }()
	return nil
}
func (s *Server) Close(ctx context.Context) error {
	var err error
	s.once.Do(func() {
		if s.server != nil {
			err = s.server.Shutdown(ctx)
		}
		_ = os.Remove(s.endpointPath)
	})
	return err
}
func (s *Server) authorize(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.RemoteAddr == "" || !strings.HasPrefix(r.RemoteAddr, "127.0.0.1:") {
			http.Error(w, "loopback only", http.StatusForbidden)
			return
		}
		if !constantTimeEqual(r.Header.Get("Authorization"), "Bearer "+s.endpoint.Token) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}
func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"version": ProtocolVersion, "ok": true, "pid": os.Getpid()})
}
func safePoolEntry(entry *services.PoolEntry) *PoolEntryStatus {
	if entry == nil {
		return nil
	}
	return &PoolEntryStatus{NodeID: entry.Node.ID, DisplayName: entry.Node.DisplayName, OriginalName: entry.OriginalName, Score: entry.Score, State: string(entry.State), Failures: entry.Failures, LastSuccess: entry.LastSuccess}
}
func safePools(pools map[string]services.Pool) map[string]NodePoolStatus {
	out := make(map[string]NodePoolStatus, len(pools))
	for name, pool := range pools {
		item := NodePoolStatus{Service: pool.Service, Primary: safePoolEntry(pool.Primary), Secondary: safePoolEntry(pool.Secondary), CandidateCount: len(pool.Candidates), UpdatedAt: pool.UpdatedAt}
		for _, entry := range pool.Standby {
			if safe := safePoolEntry(entry); safe != nil {
				item.Standby = append(item.Standby, *safe)
			}
		}
		out[name] = item
	}
	return out
}

func (s *Server) snapshot() Snapshot {
	pipeline := s.engine.Status()
	rawPools := s.engine.ServicePools()
	pools := safePools(rawPools)
	runtimes := s.engine.RuntimeStates()
	routerPorts := s.engine.RouterPorts()
	serviceStatuses := make([]ServiceStatus, 0, len(s.cfg.Services.Services))
	for name, spec := range s.cfg.Services.Services {
		status := ServiceStatus{ID: name, State: "STOPPED", RuntimeState: "NOT_STARTED"}
		pool, hasPool := rawPools[name]
		runtime, hasRuntime := runtimes[name]
		if !spec.Enabled {
			status.State = "DISABLED"
		} else if hasPool {
			status.PoolSize = len(pool.Candidates)
			for _, candidate := range pool.Candidates {
				status.PoolNodes = append(status.PoolNodes, candidate.Node.DisplayName)
			}
			status.Standby = len(pool.Standby)
			if pool.Primary == nil {
				status.State = "RECOVERING"
			} else {
				status.Primary = pool.Primary.Node.DisplayName
				if pool.Primary.Failures > 0 {
					status.State = "DEGRADED"
				} else if pool.Primary.State == services.Recovering {
					status.State = "RECOVERING"
				} else if pool.Primary.LastSuccess.IsZero() {
					status.State = "CONNECTING"
				} else {
					lastCheck := pool.Primary.LastSuccess
					status.LastHealthCheck = &lastCheck
					status.LatencyMs = int64(pool.Primary.Score)
					if hasRuntime && runtime.Running && routerPorts[name] == runtime.Port {
						status.State = "ACTIVE"
						status.Available = true
					} else {
						status.State = "CONNECTING"
						status.Message = "service probe succeeded, but its domain route is not active"
					}
				}
			}
			if pool.Secondary != nil {
				status.Secondary = pool.Secondary.Node.DisplayName
			}
		} else if pipeline == string(engine.StatusRunning) {
			status.State = "STARTING"
		} else if len(rawPools) == 0 && len(runtimes) == 0 {
			status.State = "STOPPED"
		} else if pipeline == string(engine.StatusCompleted) || pipeline == string(engine.StatusError) {
			status.State = "RECOVERING"
		}
		if hasRuntime {
			status.RuntimeState = runtime.State
			status.PID = runtime.PID
			status.Port = runtime.Port
			status.LastError = runtime.LastError
		} else if spec.Enabled && pipeline == string(engine.StatusRunning) {
			status.RuntimeState = "STARTING"
		}
		if spec.Enabled && hasPool && (!hasRuntime || !runtime.Running) {
			status.State = "RECOVERING"
			status.Available = false
		}
		if status.LastError != "" {
			status.Message = status.LastError
		}
		serviceStatuses = append(serviceStatuses, status)
	}
	sort.Slice(serviceStatuses, func(i, j int) bool { return serviceStatuses[i].ID < serviceStatuses[j].ID })
	return Snapshot{Version: ProtocolVersion, Timestamp: time.Now().UTC(), Backend: "READY", Pipeline: pipeline, Stages: s.engine.Progress().Snapshot(), Sources: s.readSources(), Services: serviceStatuses, Pools: pools, Runtimes: runtimes, Router: s.engine.RouterStatus(), Capabilities: Capabilities{PipelineControl: true, SourceListing: true, SettingsRead: true, SettingsWrite: true, ServiceManager: true, RuntimeStatus: true}}
}
func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.snapshot())
}
func (s *Server) readSources() []SourceStatus {
	data, err := os.ReadFile(filepath.Join(s.root, "configs", "sources.yaml"))
	if err != nil {
		return []SourceStatus{}
	}
	var raw struct {
		Sources []struct {
			Name    string `yaml:"name"`
			Type    string `yaml:"type"`
			URL     string `yaml:"url"`
			Enabled bool   `yaml:"enabled"`
		} `yaml:"sources"`
	}
	if yaml.Unmarshal(data, &raw) != nil {
		return []SourceStatus{}
	}
	out := make([]SourceStatus, 0, len(raw.Sources))
	for _, v := range raw.Sources {
		out = append(out, SourceStatus{Name: v.Name, Type: v.Type, URL: v.URL, Enabled: v.Enabled})
	}
	return out
}
func (s *Server) handleSources(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"version": ProtocolVersion, "sources": s.readSources()})
}
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	limit := 1000
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 2000 {
		limit = 2000
	}
	path := filepath.Join(s.root, "logs", "application.log")
	f, err := os.Open(path)
	if err != nil {
		writeJSON(w, 200, map[string]any{"version": ProtocolVersion, "lines": []string{}})
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	const maxTailBytes int64 = 2 << 20
	var source io.Reader = f
	if info.Size() > maxTailBytes {
		if _, err := f.Seek(info.Size()-maxTailBytes, io.SeekStart); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		reader := bufio.NewReader(f)
		_, _ = reader.ReadString('\n')
		source = reader
	}
	scanner := bufio.NewScanner(source)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 256*1024)
	lines := make([]string, 0, limit)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 4096 {
			line = line[:4096] + "…"
		}
		lines = append(lines, line)
		if len(lines) > limit {
			lines = lines[1:]
		}
	}
	if err := scanner.Err(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, map[string]any{"version": ProtocolVersion, "lines": lines})
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"version": ProtocolVersion, "network_mode": s.cfg.NetworkMode, "run_share_server": s.cfg.RunShareServer, "fetcher_timeout": s.cfg.Fetcher.Timeout, "home_profile": s.cfg.HomeProfile, "mobile_profile": s.cfg.MobileProfile})
}
func (s *Server) handleSetNetworkMode(w http.ResponseWriter, r *http.Request) {
	if s.engine.Status() == string(engine.StatusRunning) || s.engine.Status() == string(engine.StatusPaused) || s.engine.Status() == string(engine.StatusPausing) || s.engine.Status() == string(engine.StatusResuming) {
		http.Error(w, "stop the pipeline before changing network mode", http.StatusConflict)
		return
	}
	var body struct {
		NetworkMode string `json:"network_mode"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	mode := strings.ToLower(strings.TrimSpace(body.NetworkMode))
	if mode != "home" && mode != "mobile" {
		http.Error(w, "network_mode must be home or mobile", http.StatusBadRequest)
		return
	}
	path := filepath.Join(s.root, "configs", "settings.yaml")
	original, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var doc yaml.Node
	if err = yaml.Unmarshal(original, &doc); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if len(doc.Content) == 0 {
		http.Error(w, "empty settings document", 500)
		return
	}
	root := doc.Content[0]
	found := false
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "network_mode" {
			root.Content[i+1].Value = mode
			found = true
			break
		}
	}
	if !found {
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "network_mode"}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: mode})
	}
	encoded, err := yaml.Marshal(&doc)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var fresh config.Config
	if err = yaml.Unmarshal(encoded, &fresh); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	fresh.Services = s.cfg.Services
	fresh.Recovery = s.cfg.Recovery
	if err = fresh.Validate(); err != nil {
		http.Error(w, "setting validation failed: "+err.Error(), http.StatusBadRequest)
		return
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, encoded, 0600); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err = os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		http.Error(w, err.Error(), 500)
		return
	}
	s.cfg.NetworkMode = fresh.NetworkMode
	writeJSON(w, 200, map[string]any{"version": ProtocolVersion, "network_mode": mode, "requires_pipeline_restart": true})
}
func (s *Server) handleCommand(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	var c Command
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeJSON(w, 400, CommandResult{Version: ProtocolVersion, Accepted: false, Status: "ERROR", Error: "invalid JSON command"})
		return
	}
	if c.Version != ProtocolVersion {
		writeJSON(w, 400, CommandResult{Version: ProtocolVersion, ID: c.ID, Accepted: false, Status: "ERROR", Error: "unsupported protocol version"})
		return
	}
	res := CommandResult{Version: ProtocolVersion, ID: c.ID, Accepted: true, Status: "ACCEPTED"}
	switch strings.ToLower(c.Action) {
	case "get-status":
		res.Status = "SNAPSHOT"
	case "start":
		err := s.engine.Start(context.Background())
		if err != nil {
			res.Accepted = false
			res.Status = "ERROR"
			res.Error = err.Error()
		} else {
			res.Status = "STARTED"
		}
	case "pause":
		if s.engine.Status() != string(engine.StatusRunning) {
			res.Accepted = false
			res.Status = "ERROR"
			res.Error = "pipeline is not running"
		} else {
			s.engine.Pause()
			res.Status = "PAUSED"
		}
	case "continue", "resume":
		if s.engine.Status() != string(engine.StatusPaused) {
			res.Accepted = false
			res.Status = "ERROR"
			res.Error = "pipeline is not paused"
		} else {
			s.engine.Resume()
			res.Status = "RUNNING"
		}
	case "stop":
		wasRunning := s.engine.Status() == string(engine.StatusRunning) || s.engine.Status() == string(engine.StatusPaused) || s.engine.Status() == string(engine.StatusPausing) || s.engine.Status() == string(engine.StatusResuming)
		s.engine.Stop()
		if wasRunning {
			res.Status = "STOP_REQUESTED"
		} else {
			res.Status = "SERVICES_STOPPED"
		}
	case "start-service", "reconnect-service":
		if c.ServiceID == "" {
			res.Accepted = false
			res.Status = "ERROR"
			res.Error = "service_id is required"
		} else if err := s.engine.RestartService(r.Context(), c.ServiceID); err != nil {
			res.Accepted = false
			res.Status = "ERROR"
			res.Error = err.Error()
		} else {
			res.Status = "RESTARTED"
		}
	case "get-pool", "get-runtime":
		res.Status = "SNAPSHOT"
	case "stop-service":
		res.Accepted = false
		res.Status = "UNSUPPORTED"
		res.Error = "stopping one service is disabled until routing can safely remove only that service's route; use full pipeline STOP to stop all managed runtimes"
	default:
		res.Accepted = false
		res.Status = "ERROR"
		res.Error = fmt.Sprintf("unknown action %q", c.Action)
	}
	res.SnapshotPtr(s)
	status := 200
	if !res.Accepted {
		status = 409
	}
	writeJSON(w, status, res)
}
func (r *CommandResult) SnapshotPtr(s *Server) { snap := s.snapshot(); r.Snapshot = &snap }
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	ch, unsubscribe := s.engine.Events().Subscribe(64)
	defer unsubscribe()
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()
	send := func(kind string, v any) bool {
		b, err := json.Marshal(v)
		if err != nil {
			return true
		}
		if _, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, b); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !send("snapshot", s.snapshot()) {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			if !send("backend-event", map[string]any{"version": ProtocolVersion, "timestamp": time.Now().UTC(), "event": e, "snapshot": s.snapshot()}) {
				return
			}
		case <-ticker.C:
			if !send("snapshot", s.snapshot()) {
				return
			}
		}
	}
}
