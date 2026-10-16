package engine

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/checker"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/config"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/core/collector"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/core/control"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/core/events"
	coreprocessor "github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/core/processor"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/fetcher"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/logger"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/model"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/processor"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/progress"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/routing"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/services"
	"go.uber.org/zap"
)

type Status string

const (
	StatusIdle      Status = "IDLE"
	StatusRunning   Status = "RUNNING"
	StatusPausing   Status = "PAUSING"
	StatusPaused    Status = "PAUSED"
	StatusResuming  Status = "RESUMING"
	StatusCompleted Status = "COMPLETED"
	StatusError     Status = "ERROR"
)

type Controller interface {
	Start(context.Context) error
	Stop()
	Pause()
	Resume()
	Status() string
	Progress() *progress.Manager
	Events() *events.Bus
}
type Engine struct {
	cfg            *config.Config
	root           string
	pm             *progress.Manager
	bus            *events.Bus
	gate           *control.PauseGate
	mu             sync.RWMutex
	status         Status
	cancel         context.CancelFunc
	running        bool
	serviceManager *services.Manager
	router         *routing.Router
}

func New(cfg *config.Config, root string) *Engine {
	return &Engine{cfg: cfg, root: root, pm: progress.NewManager(), bus: events.NewBus(), gate: control.NewPauseGate(), status: StatusIdle}
}
func (e *Engine) ValidateRuntime() error {
	if e.cfg == nil {
		return errors.New("nil config")
	}
	p := e.singboxPath()
	if _, err := os.Stat(p); err != nil {
		return fmt.Errorf("sing-box-lx not found: %w", err)
	}
	return nil
}
func (e *Engine) Progress() *progress.Manager { return e.pm }
func (e *Engine) Events() *events.Bus         { return e.bus }
func (e *Engine) Status() string              { e.mu.RLock(); defer e.mu.RUnlock(); return string(e.status) }
func (e *Engine) Start(parent context.Context) error {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return errors.New("engine already running")
	}
	oldServices := e.serviceManager
	oldRouter := e.router
	e.serviceManager = nil
	e.router = nil
	ctx, cancel := context.WithCancel(parent)
	e.cancel = cancel
	e.running = true
	e.status = StatusRunning
	e.gate.Resume()
	e.mu.Unlock()
	if oldServices != nil {
		oldServices.Stop()
	}
	if oldRouter != nil {
		oldRouter.Stop()
	}
	e.bus.Publish(events.Event{Type: events.Start})
	go e.run(ctx)
	return nil
}
func (e *Engine) Stop() {
	e.mu.Lock()
	c := e.cancel
	sm := e.serviceManager
	r := e.router
	e.serviceManager = nil
	e.router = nil
	e.mu.Unlock()
	if c != nil {
		c()
	}
	if sm != nil {
		sm.Stop()
	}
	if r != nil {
		r.Stop()
	}
}
func (e *Engine) Pause() {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return
	}
	e.status = StatusPausing
	e.mu.Unlock()
	e.gate.Pause()
	e.pm.PauseAll()
	e.mu.Lock()
	e.status = StatusPaused
	e.mu.Unlock()
	e.bus.Publish(events.Event{Type: events.Pause})
}
func (e *Engine) Resume() {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return
	}
	e.status = StatusResuming
	e.mu.Unlock()
	e.gate.Resume()
	e.pm.ResumeAll()
	e.mu.Lock()
	e.status = StatusRunning
	e.mu.Unlock()
	e.bus.Publish(events.Event{Type: events.Resume})
}
func (e *Engine) finish(status Status) {
	e.mu.Lock()
	e.running = false
	e.cancel = nil
	e.status = status
	e.mu.Unlock()
	e.bus.Publish(events.Event{Type: events.Complete})
}
func (e *Engine) run(ctx context.Context) {
	err := e.pipeline(ctx)
	if err != nil && ctx.Err() == nil {
		if logger.Log != nil {
			logger.Log.Error("pipeline failed", zap.Error(err))
		}
		e.bus.Publish(events.Event{Type: events.Error, Message: err.Error()})
		e.finish(StatusError)
		return
	}
	if ctx.Err() != nil {
		e.finish(StatusIdle)
		return
	}
	e.finish(StatusCompleted)
}
func (e *Engine) pipeline(ctx context.Context) error {
	if len(e.cfg.Services.Services) == 0 {
		if err := e.cfg.LoadRuntimeFiles(e.root); err != nil {
			return err
		}
	}
	mode := normalizeMode(e.cfg.NetworkMode)
	profile := e.cfg.GetActiveProfile()
	servicesMap := map[string]string{}
	for _, name := range profile.EnabledServices {
		if spec, ok := e.cfg.Services.Services[name]; ok && spec.ProbeURL != "" {
			servicesMap[name] = spec.ProbeURL
		}
	}
	if len(servicesMap) == 0 {
		return errors.New("no active services configured")
	}
	e.pm.AddStage("FETCHER", progress.UnknownTotal, profile.FetcherWorkers)
	e.pm.AddStage("COLLECTOR", progress.UnknownTotal, 1)
	e.pm.AddStage("PROCESSOR", progress.UnknownTotal, 1)
	e.pm.AddStage("PARSER", progress.UnknownTotal, profile.ParseWorkers)
	e.pm.AddStage("TCP CHECK", progress.UnknownTotal, profile.TCPWorkers)
	e.pm.AddStage("DEEP CHECK", progress.UnknownTotal, e.cfg.GetDeepWorkersCount())
	sources, err := config.LoadSources(filepath.Join(e.root, "configs", "sources.yaml"))
	if err != nil {
		return err
	}
	dedup := processor.NewConcurrentDedup()
	raw := fetcher.New(e.cfg, sources, e.pm, e.gate).StreamFetch(ctx, dedup)
	collected := collector.Stream(ctx, raw, e.pm)
	processed := coreprocessor.Stream(ctx, collected, e.pm)
	parsed := checker.StreamParseNodes(ctx, processed, profile.ParseWorkers, e.pm, e.gate)
	tcp := checker.StreamTCPCheck(ctx, parsed, time.Duration(profile.TCPTimeoutMs)*time.Millisecond, profile.TCPWorkers, e.pm, e.gate)
	existing := make(chan model.NodeInfo, 1024)
	existingNodes, err := loadExistingPool(filepath.Join(e.root, "output", "subscriptions", "all_"+mode+".txt"))
	if err != nil {
		return err
	}
	go func() {
		defer close(existing)
		for _, n := range existingNodes {
			if e.gate.Wait(ctx) != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case existing <- n:
			}
		}
	}()
	candidates := mergeNodeStreams(ctx, existing, tcp)
	deep := checker.NewDeepChecker(e.singboxPath(), filepath.Join(e.root, "data", "cache", "deep-workers"), e.cfg.GetDeepWorkersCount(), time.Duration(profile.DeepTimeoutSec)*time.Second, servicesMap)
	results := deep.StreamDeepCheck(ctx, candidates, e.pm, e.gate)
	ranked, err := checker.PipelineConsumer(ctx, results, filepath.Join(e.root, "output", "subscriptions"), mode, profile.MaxOutputLimit, profile.StableOutputLimit, filepath.Join(e.root, "data", "history"), e.pm)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	router := routing.New(e.root, e.singboxPath(), e.cfg)
	sm := services.NewManager(e.root, e.singboxPath(), e.cfg)
	sm.SetRecovery(func(rctx context.Context, serviceName string) ([]model.FinalResult, error) {
		return e.recoverService(rctx, serviceName, sources)
	})
	// Only route to service ports whose runtime has passed its own probe. Runtime
	// changes re-apply the router with the currently healthy ports, so a failed
	// service cannot black-hole that domain or force unrelated services offline.
	sm.SetRuntimeChange(func() {
		ports := sm.RuntimePorts()
		if len(ports) == 0 {
			router.Stop()
			return
		}
		if err := router.Apply(ctx, ports); err != nil && logger.Log != nil {
			logger.Log.Warn("domain router update failed", zap.Error(err))
		}
	})
	e.mu.Lock()
	if err := ctx.Err(); err != nil {
		e.mu.Unlock()
		sm.Stop()
		router.Stop()
		return err
	}
	e.serviceManager = sm
	e.router = router
	startErr := sm.Start(ctx, ranked)
	e.mu.Unlock()
	if startErr != nil && logger.Log != nil {
		logger.Log.Warn("service manager startup incomplete", zap.Error(startErr))
	}
	if e.cfg.RunShareServer {
		return e.startShareServer()
	}
	return nil
}
func (e *Engine) singboxPath() string {
	return filepath.Join(e.root, "tools", "sing-box-lx", "sing-box.exe")
}
func (e *Engine) startShareServer() error {
	args := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", filepath.Join(e.root, "cmd", "vpn-manager", "share_server.ps1")}
	cmd := exec.Command("powershell", args...)
	cmd.Dir = e.root
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd.Start()
}
func mergeNodeStreams(ctx context.Context, a, b <-chan model.NodeInfo) <-chan model.NodeInfo {
	out := make(chan model.NodeInfo, 2048)
	dedup := processor.NewConcurrentDedup()
	go func() {
		defer close(out)
		for a != nil || b != nil {
			select {
			case <-ctx.Done():
				return
			case n, ok := <-a:
				if !ok {
					a = nil
					continue
				}
				if !dedup.IsUnique(n.Raw) {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case out <- n:
				}
			case n, ok := <-b:
				if !ok {
					b = nil
					continue
				}
				if !dedup.IsUnique(n.Raw) {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case out <- n:
				}
			}
		}
	}()
	return out
}
func loadExistingPool(path string) ([]model.NodeInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []model.NodeInfo
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 32*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		if n, err := checker.ParseNode(line); err == nil {
			out = append(out, n)
		}
	}
	return out, sc.Err()
}
func normalizeMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), "mobile") {
		return "mobile"
	}
	return "home"
}

func (e *Engine) recoverService(ctx context.Context, serviceName string, allSources *config.SourcesConfig) ([]model.FinalResult, error) {
	spec, ok := e.cfg.Services.Services[serviceName]
	if !ok {
		return nil, fmt.Errorf("unknown recovery service %s", serviceName)
	}
	profile := e.cfg.GetActiveProfile()
	maxSources := e.cfg.Recovery.MaxSourcesPerCycle
	if maxSources <= 0 {
		maxSources = 8
	}
	selected := make([]config.Source, 0, maxSources)
	for _, src := range allSources.Sources {
		if !src.Enabled {
			continue
		}
		selected = append(selected, src)
		if len(selected) >= maxSources {
			break
		}
	}
	if len(selected) == 0 {
		return nil, errors.New("no recovery sources")
	}
	local := &config.SourcesConfig{Sources: selected}
	raw := fetcher.New(e.cfg, local, nil, e.gate).StreamFetch(ctx, nil)
	collected := collector.Stream(ctx, raw, nil)
	processed := coreprocessor.Stream(ctx, collected, nil)
	parsed := checker.StreamParseNodes(ctx, processed, profile.ParseWorkers, nil, e.gate)
	tcp := checker.StreamTCPCheck(ctx, parsed, time.Duration(profile.TCPTimeoutMs)*time.Millisecond, profile.TCPWorkers, nil, e.gate)
	svc := map[string]string{serviceName: spec.ProbeURL}
	deep := checker.NewDeepChecker(e.singboxPath(), filepath.Join(e.root, "data", "cache", "recovery", serviceName), 1, time.Duration(profile.DeepTimeoutSec)*time.Second, svc)
	results := deep.StreamDeepCheck(ctx, tcp, nil, e.gate)
	maxNodes := e.cfg.Recovery.MaxWorkingNodes
	if maxNodes <= 0 {
		maxNodes = 12
	}
	out := make([]model.FinalResult, 0, maxNodes)
	for {
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case r, ok := <-results:
			if !ok {
				return out, nil
			}
			if r.Alive && r.SuccessServices[serviceName] {
				out = append(out, r)
				if len(out) >= maxNodes {
					return out, nil
				}
			}
		}
	}
}

func (e *Engine) RestartService(ctx context.Context, name string) error {
	e.mu.RLock()
	manager := e.serviceManager
	e.mu.RUnlock()
	if manager == nil {
		return errors.New("service manager is not initialized; run a completed check first")
	}
	return manager.RestartService(ctx, name)
}

func (e *Engine) ServicePools() map[string]services.Pool {
	e.mu.RLock()
	manager := e.serviceManager
	e.mu.RUnlock()
	if manager == nil {
		return map[string]services.Pool{}
	}
	return manager.Snapshot()
}

func (e *Engine) RuntimeStates() map[string]services.RuntimeStatus {
	e.mu.RLock()
	manager := e.serviceManager
	e.mu.RUnlock()
	if manager == nil {
		return map[string]services.RuntimeStatus{}
	}
	return manager.RuntimeSnapshot()
}

func (e *Engine) RouterPorts() map[string]int {
	e.mu.RLock()
	router := e.router
	e.mu.RUnlock()
	if router == nil {
		return map[string]int{}
	}
	return router.Ports()
}

func (e *Engine) RouterStatus() string {
	e.mu.RLock()
	router := e.router
	e.mu.RUnlock()
	if router == nil {
		return "NOT_STARTED"
	}
	return router.Status()
}

func (e *Engine) RuntimeStats() map[string]int {
	out := map[string]int{"activeServices": 0, "recoveringServices": 0, "runningLX": 0}
	e.mu.RLock()
	manager := e.serviceManager
	e.mu.RUnlock()
	if manager != nil {
		running := manager.RuntimePorts()
		routed := e.RouterPorts()
		for name, pool := range manager.Snapshot() {
			port, hasRuntime := running[name]
			if hasRuntime && routed[name] == port && pool.Primary != nil && !pool.Primary.LastSuccess.IsZero() && pool.Primary.Failures == 0 {
				out["activeServices"]++
			} else {
				out["recoveringServices"]++
			}
		}
		out["runningLX"] = len(running)
	}
	return out
}
