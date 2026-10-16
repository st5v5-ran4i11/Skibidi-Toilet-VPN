package routing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/checker"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/config"
	lxruntime "github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/runtime"
)

type Router struct {
	root    string
	binary  string
	cfg     *config.Config
	lx      *lxruntime.LXD
	mu      sync.Mutex
	running bool
	ports   map[string]int
}

func New(root, binary string, cfg *config.Config) *Router {
	return &Router{root: root, binary: binary, cfg: cfg}
}

func (r *Router) Apply(ctx context.Context, ports map[string]int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(ports) == 0 {
		return errors.New("no active service ports for router")
	}
	services := map[string]struct {
		Port    int
		Domains []string
	}{}
	for name, port := range ports {
		spec, ok := r.cfg.Services.Services[name]
		if !ok || !spec.Enabled {
			continue
		}
		services[name] = struct {
			Port    int
			Domains []string
		}{Port: port, Domains: spec.Domains}
	}
	data, err := checker.MarshalRouterConfig(services)
	if err != nil {
		return err
	}
	state := filepath.Join(r.root, "data", "runtime", "router")
	_ = os.MkdirAll(state, 0755)
	createdRuntime := false
	if r.lx == nil {
		cp, err := lxruntime.FreeTCPPort()
		if err != nil {
			return err
		}
		lx := lxruntime.NewLXD(r.binary, state)
		lx.ControlPort = cp
		if err := lx.Start(ctx, nil); err != nil {
			_ = lx.Stop()
			return err
		}
		r.lx = lx
		createdRuntime = true
	}
	if err := r.lx.Apply(ctx, data); err != nil {
		if createdRuntime {
			_ = r.lx.Stop()
			r.lx = nil
		}
		return err
	}
	_ = os.WriteFile(filepath.Join(state, "config.json"), data, 0600)
	r.ports = make(map[string]int, len(ports))
	for name, port := range ports {
		r.ports[name] = port
	}
	r.running = true
	return nil
}

func (r *Router) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lx != nil {
		_ = r.lx.Stop()
		r.lx = nil
	}
	r.running = false
	r.ports = map[string]int{}
}
func (r *Router) Status() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return fmt.Sprintf("RUNNING pid=%d", r.lx.Process.Process.Pid)
	}
	return "STOPPED"
}

func (r *Router) Ports() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]int, len(r.ports))
	for name, port := range r.ports {
		out[name] = port
	}
	return out
}
