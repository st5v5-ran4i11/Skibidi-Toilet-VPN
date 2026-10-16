package services

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/analyzer"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/checker"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/config"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/core/naming"
	coreranking "github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/core/ranking"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/model"
	lxruntime "github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/runtime"
)

type State string

const (
	Active     State = "ACTIVE"
	Suspect    State = "SUSPECT"
	Quarantine State = "QUARANTINE"
	Dead       State = "DEAD"
	Recovering State = "RECOVERING"
	Inactive   State = "INACTIVE"
)

type PoolEntry struct {
	Node         model.NodeInfo `json:"node"`
	OriginalName string         `json:"original_name,omitempty"`
	Score        float64        `json:"score"`
	State        State          `json:"state"`
	Failures     int            `json:"failures"`
	LastSuccess  time.Time      `json:"last_success"`
}

type Pool struct {
	Service    string       `json:"service"`
	Primary    *PoolEntry   `json:"primary,omitempty"`
	Secondary  *PoolEntry   `json:"secondary,omitempty"`
	Standby    []*PoolEntry `json:"standby,omitempty"`
	Candidates []*PoolEntry `json:"candidates,omitempty"`
	UpdatedAt  time.Time    `json:"updated_at"`
}

type RuntimeStatus struct {
	Service    string `json:"service"`
	State      string `json:"state"`
	PID        int    `json:"pid"`
	Port       int    `json:"port"`
	Running    bool   `json:"running"`
	ConfigPath string `json:"config_path,omitempty"`
	LastError  string `json:"last_error,omitempty"`
}

type Runtime struct {
	Service    string
	Port       int
	LXD        *lxruntime.LXD
	ConfigPath string
	PID        int
	Running    bool
	LastError  string
}

type Manager struct {
	root            string
	binary          string
	cfg             *config.Config
	mu              sync.RWMutex
	pools           map[string]*Pool
	runtimes        map[string]*Runtime
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	recover         func(context.Context, string) ([]model.FinalResult, error)
	analyzer        *analyzer.Analyzer
	onRuntimeChange func()
}

func servicePorts(name string) (int, int) {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	base := int(h.Sum32() % 700)
	return 21000 + base*2, 31000 + base*2
}

func NewManager(root, binary string, cfg *config.Config) *Manager {
	return &Manager{root: root, binary: binary, cfg: cfg, pools: map[string]*Pool{}, runtimes: map[string]*Runtime{}, analyzer: analyzer.New(analyzer.Passive, 5000)}
}

func (m *Manager) SetRecovery(fn func(context.Context, string) ([]model.FinalResult, error)) {
	m.recover = fn
}

func (m *Manager) SetRuntimeChange(fn func()) { m.onRuntimeChange = fn }
func (m *Manager) notifyRuntimeChange() {
	if m.onRuntimeChange != nil {
		m.onRuntimeChange()
	}
}

func (m *Manager) replacePool(name string, results []model.FinalResult) {
	spec, ok := m.cfg.Services.Services[name]
	if !ok {
		return
	}
	m.bootstrapService(context.Background(), name, spec, results)
}

func (m *Manager) Start(ctx context.Context, results []model.FinalResult) error {
	if m.cfg == nil {
		return errors.New("service manager config is nil")
	}
	ctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	for name, spec := range m.cfg.Services.Services {
		if !spec.Enabled {
			continue
		}
		m.bootstrapService(ctx, name, spec, results)
	}
	m.wg.Add(1)
	go func() { defer m.wg.Done(); m.maintenance(ctx) }()
	return nil
}

func (m *Manager) bootstrapService(ctx context.Context, name string, spec config.ServiceConfig, results []model.FinalResult) {
	m.mu.Lock()
	p := &Pool{Service: name, UpdatedAt: time.Now()}
	ranked := coreranking.RankService(results, name)
	for _, r := range ranked {
		if !r.Success {
			continue
		}
		if len(p.Candidates) >= spec.TargetNodes+spec.StandbyNodes {
			break
		}
		node := r.Node
		originalName := naming.OriginalDisplayName(node.DisplayName)
		node.DisplayName = naming.ServiceDisplayName(name, originalName)
		p.Candidates = append(p.Candidates, &PoolEntry{Node: node, OriginalName: originalName, Score: r.Score, State: Active})
	}
	if len(p.Candidates) == 0 {
		for _, r := range results {
			if len(p.Candidates) >= spec.TargetNodes+spec.StandbyNodes {
				break
			}
			node := r.Node
			originalName := naming.OriginalDisplayName(node.DisplayName)
			node.DisplayName = naming.ServiceDisplayName(name, originalName)
			p.Candidates = append(p.Candidates, &PoolEntry{Node: node, OriginalName: originalName, Score: float64(r.Score), State: Active})
		}
	}
	sort.SliceStable(p.Candidates, func(i, j int) bool { return p.Candidates[i].Score > p.Candidates[j].Score })
	if len(p.Candidates) > 0 {
		p.Primary = p.Candidates[0]
	}
	if len(p.Candidates) > 1 {
		p.Secondary = p.Candidates[1]
	}
	start := 2
	end := start + spec.StandbyNodes
	if end > len(p.Candidates) {
		end = len(p.Candidates)
	}
	if end > start {
		p.Standby = append([]*PoolEntry(nil), p.Candidates[start:end]...)
	}
	m.pools[name] = p
	m.mu.Unlock()
	m.wg.Add(1)
	go func() { defer m.wg.Done(); _ = m.ensureRuntime(ctx, name, spec) }()
}

func (m *Manager) ensureRuntime(ctx context.Context, name string, spec config.ServiceConfig) error {
	m.mu.RLock()
	p := m.pools[name]
	var primaryNode model.NodeInfo
	hasPrimary := p != nil && p.Primary != nil
	if hasPrimary {
		primaryNode = p.Primary.Node
		primaryNode.Sources = append([]string(nil), p.Primary.Node.Sources...)
	}
	m.mu.RUnlock()
	if !hasPrimary {
		m.setState(name, Recovering)
		return errors.New("no primary candidate")
	}
	port, controlPort := servicePorts(name)
	m.mu.Lock()
	oldRuntime := m.runtimes[name]
	if oldRuntime != nil {
		oldRuntime.Running = false
		if oldRuntime.LXD != nil {
			_ = oldRuntime.LXD.Stop()
		}
	}
	m.mu.Unlock()
	m.notifyRuntimeChange()
	stateDir := filepath.Join(m.root, "data", "runtime", name)
	_ = os.MkdirAll(stateDir, 0755)
	lx := lxruntime.NewLXD(m.binary, stateDir)
	lx.ControlPort = controlPort
	cfg, err := checker.MarshalServiceConfig(primaryNode, port)
	if err != nil {
		return err
	}
	if err := lx.Start(ctx, nil); err != nil {
		m.mu.Lock()
		m.runtimes[name] = &Runtime{Service: name, Port: port, LXD: lx, PID: processPID(lx), Running: false, ConfigPath: filepath.Join(stateDir, "config.json"), LastError: err.Error()}
		m.mu.Unlock()
		m.setState(name, Recovering)
		m.notifyRuntimeChange()
		return err
	}
	if err := lx.Apply(ctx, cfg); err != nil {
		_ = lx.Stop()
		m.mu.Lock()
		m.runtimes[name] = &Runtime{Service: name, Port: port, LXD: lx, PID: processPID(lx), Running: false, ConfigPath: filepath.Join(stateDir, "config.json"), LastError: err.Error()}
		m.mu.Unlock()
		m.setState(name, Recovering)
		m.notifyRuntimeChange()
		return err
	}
	// A started process is not considered usable until a request has passed through
	// its local proxy port to the service-specific probe URL.
	var ready bool
	var latency time.Duration
	var probeErr error
	for attempt := 0; attempt < 3; attempt++ {
		probeCtx, cancelProbe := context.WithTimeout(ctx, 4*time.Second)
		ready, latency = probeThroughPort(probeCtx, port, spec.ProbeURL)
		cancelProbe()
		if ready {
			break
		}
		if ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
			attempt = 3
		case <-time.After(500 * time.Millisecond):
		}
	}
	if !ready {
		_ = lx.Stop()
		probeErr = fmt.Errorf("runtime started but service probe failed for %s", spec.ProbeURL)
		m.mu.Lock()
		m.runtimes[name] = &Runtime{Service: name, Port: port, LXD: lx, PID: processPID(lx), Running: false, ConfigPath: filepath.Join(stateDir, "config.json"), LastError: probeErr.Error()}
		m.mu.Unlock()
		m.setState(name, Recovering)
		m.notifyRuntimeChange()
		return probeErr
	}
	m.mu.Lock()
	m.runtimes[name] = &Runtime{Service: name, Port: port, LXD: lx, PID: processPID(lx), Running: true, ConfigPath: filepath.Join(stateDir, "config.json")}
	if current := m.pools[name]; current != nil && current.Primary != nil {
		current.Primary.LastSuccess = time.Now()
		current.Primary.Failures = 0
		current.Primary.Score = float64(latency.Milliseconds())
		current.Primary.State = Active
		current.UpdatedAt = time.Now()
	}
	m.mu.Unlock()
	_ = m.writeRuntimeConfig(name, cfg)
	m.notifyRuntimeChange()
	return nil
}

func processPID(lx *lxruntime.LXD) int {
	if lx != nil && lx.Process != nil && lx.Process.Process != nil {
		return lx.Process.Process.Pid
	}
	return 0
}

func (m *Manager) maintenance(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.healthTick(ctx)
		}
	}
}

func (m *Manager) healthTick(ctx context.Context) {
	m.mu.RLock()
	names := make([]string, 0, len(m.pools))
	for n := range m.pools {
		names = append(names, n)
	}
	m.mu.RUnlock()
	for _, name := range names {
		spec, ok := m.cfg.Services.Services[name]
		if !ok {
			continue
		}
		m.mu.RLock()
		p := m.pools[name]
		rt := m.runtimes[name]
		m.mu.RUnlock()
		if p == nil || p.Primary == nil || rt == nil || !rt.Running || rt.LXD == nil || rt.LXD.Process == nil {
			m.setState(name, Recovering)
			if m.recover != nil {
				results, err := m.recover(ctx, name)
				if err == nil && len(results) > 0 {
					m.replacePool(name, results)
				}
			}
			continue
		}
		ok, lat := probeThroughPort(ctx, rt.Port, spec.ProbeURL)
		if m.analyzer != nil {
			nodeID := ""
			if p.Primary != nil {
				nodeID = p.Primary.Node.ID
			}
			m.analyzer.Record(analyzer.Event{Service: name, Domain: spec.Domains[0], NodeID: nodeID, Latency: lat, Success: ok, Timeout: !ok})
		}
		if ok {
			m.mu.Lock()
			if p.Primary != nil {
				p.Primary.Failures = 0
				p.Primary.LastSuccess = time.Now()
				p.Primary.Score = float64(lat.Milliseconds())
			}
			p.UpdatedAt = time.Now()
			m.mu.Unlock()
			continue
		}
		m.failover(ctx, name, spec)
	}
}

func probeThroughPort(ctx context.Context, port int, target string) (bool, time.Duration) {
	start := time.Now()
	u, err := url.Parse(target)
	if err != nil {
		return false, 0
	}
	proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	tr := &http.Transport{Proxy: http.ProxyURL(proxyURL), DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 7 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return false, 0
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, time.Since(start)
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 400, time.Since(start)
}

func (m *Manager) failover(ctx context.Context, name string, spec config.ServiceConfig) {
	m.mu.Lock()
	p := m.pools[name]
	if p == nil || p.Primary == nil {
		m.mu.Unlock()
		return
	}
	p.Primary.Failures++
	p.Primary.State = Suspect
	p.UpdatedAt = time.Now()
	threshold := 3
	if p.Primary.Failures < threshold {
		m.mu.Unlock()
		return
	}
	var next *PoolEntry
	if p.Secondary != nil {
		next = p.Secondary
	} else if len(p.Standby) > 0 {
		next = p.Standby[0]
	}
	if next == nil {
		p.Primary.State = Recovering
		rt := m.runtimes[name]
		m.mu.Unlock()
		if rt != nil {
			if rt.LXD != nil {
				_ = rt.LXD.Stop()
			}
			m.mu.Lock()
			if current := m.runtimes[name]; current != nil {
				current.Running = false
				current.LastError = "health-check failed repeatedly and no standby node is available"
			}
			m.mu.Unlock()
		}
		m.notifyRuntimeChange()
		return
	}
	old := p.Primary
	p.Primary = next
	p.Secondary = old
	next.State = Active
	old.State = Quarantine
	p.UpdatedAt = time.Now()
	m.mu.Unlock()
	if err := m.rebindRuntime(ctx, name, spec, next.Node); err != nil {
		m.mu.Lock()
		rt := m.runtimes[name]
		if rt != nil {
			rt.Running = false
			rt.LastError = err.Error()
		}
		m.mu.Unlock()
		if rt != nil && rt.LXD != nil {
			_ = rt.LXD.Stop()
		}
		m.setState(name, Recovering)
		m.notifyRuntimeChange()
	}
}

func (m *Manager) rebindRuntime(ctx context.Context, name string, spec config.ServiceConfig, node model.NodeInfo) error {
	m.mu.RLock()
	rt := m.runtimes[name]
	m.mu.RUnlock()
	if rt == nil {
		return errors.New("runtime missing")
	}
	cfg, err := checker.MarshalServiceConfig(node, rt.Port)
	if err != nil {
		return err
	}
	if err := rt.LXD.Apply(ctx, cfg); err != nil {
		m.setError(name, err)
		m.setState(name, Recovering)
		return err
	}
	probeCtx, cancelProbe := context.WithTimeout(ctx, 5*time.Second)
	ok, latency := probeThroughPort(probeCtx, rt.Port, spec.ProbeURL)
	cancelProbe()
	if !ok {
		err := fmt.Errorf("failover node did not pass service probe for %s", spec.ProbeURL)
		m.setState(name, Recovering)
		return err
	}
	_ = m.writeRuntimeConfig(name, cfg)
	m.mu.Lock()
	if pool := m.pools[name]; pool != nil && pool.Primary != nil {
		pool.Primary.LastSuccess = time.Now()
		pool.Primary.Failures = 0
		pool.Primary.Score = float64(latency.Milliseconds())
		pool.Primary.State = Active
		pool.UpdatedAt = time.Now()
	}
	m.mu.Unlock()
	m.setState(name, Active)
	m.notifyRuntimeChange()
	return nil
}

// RestartService rebuilds the runtime for an existing configured pool. It reuses
// the deterministic local proxy port, so the existing router rule remains valid.
func (m *Manager) RestartService(ctx context.Context, name string) error {
	spec, ok := m.cfg.Services.Services[name]
	if !ok || !spec.Enabled {
		return fmt.Errorf("service %q is not enabled", name)
	}
	m.mu.RLock()
	pool := m.pools[name]
	hasPrimary := pool != nil && pool.Primary != nil
	m.mu.RUnlock()
	if !hasPrimary {
		return fmt.Errorf("service %q has no primary node in its pool", name)
	}
	return m.ensureRuntime(ctx, name, spec)
}

func (m *Manager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
	m.mu.Lock()
	for _, rt := range m.runtimes {
		if rt != nil && rt.LXD != nil {
			_ = rt.LXD.Stop()
		}
	}
	m.runtimes = map[string]*Runtime{}
	m.mu.Unlock()
	m.notifyRuntimeChange()
}

func (m *Manager) Snapshot() map[string]Pool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := map[string]Pool{}
	for k, v := range m.pools {
		if v == nil {
			continue
		}
		copyPool := Pool{Service: v.Service, UpdatedAt: v.UpdatedAt}
		copyPool.Primary = clonePoolEntry(v.Primary)
		copyPool.Secondary = clonePoolEntry(v.Secondary)
		for _, entry := range v.Standby {
			copyPool.Standby = append(copyPool.Standby, clonePoolEntry(entry))
		}
		for _, entry := range v.Candidates {
			copyPool.Candidates = append(copyPool.Candidates, clonePoolEntry(entry))
		}
		out[k] = copyPool
	}
	return out
}

func clonePoolEntry(entry *PoolEntry) *PoolEntry {
	if entry == nil {
		return nil
	}
	copyEntry := *entry
	copyEntry.Node.Sources = append([]string(nil), entry.Node.Sources...)
	return &copyEntry
}

func (m *Manager) RuntimeSnapshot() map[string]RuntimeStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]RuntimeStatus, len(m.runtimes))
	for name, rt := range m.runtimes {
		if rt == nil {
			continue
		}
		state := "STOPPED"
		if rt.Running {
			state = "RUNNING"
		}
		if rt.LastError != "" && !rt.Running {
			state = "FAILED"
		}
		out[name] = RuntimeStatus{Service: name, State: state, PID: rt.PID, Port: rt.Port, Running: rt.Running, ConfigPath: rt.ConfigPath, LastError: rt.LastError}
	}
	return out
}
func (m *Manager) setState(name string, state State) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.pools[name]; p != nil {
		if p.Primary != nil {
			p.Primary.State = state
		}
		p.UpdatedAt = time.Now()
	}
}
func (m *Manager) setError(name string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r := m.runtimes[name]; r != nil {
		r.LastError = err.Error()
	}
}
func (m *Manager) writeRuntimeConfig(name string, b []byte) error {
	path := filepath.Join(m.root, "data", "runtime", name, "config.json")
	return os.WriteFile(path, b, 0600)
}

func (m *Manager) RuntimePorts() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := map[string]int{}
	for name, rt := range m.runtimes {
		if rt != nil && rt.Running {
			out[name] = rt.Port
		}
	}
	return out
}

func (m *Manager) TrafficSnapshot() []analyzer.Event {
	if m.analyzer == nil {
		return nil
	}
	return m.analyzer.Snapshot()
}

func (m *Manager) ConfiguredPorts() map[string]int {
	out := map[string]int{}
	for name, spec := range m.cfg.Services.Services {
		if spec.Enabled {
			port, _ := servicePorts(name)
			out[name] = port
		}
	}
	return out
}
