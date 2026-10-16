package checker

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/core/control"
	coreranking "github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/core/ranking"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/logger"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/model"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/progress"
	lxruntime "github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/runtime"
	"go.uber.org/zap"
)

type DeepChecker struct {
	Binary         string
	WorkDir        string
	Workers        int
	Timeout        time.Duration
	ActiveServices map[string]string
}

func NewDeepChecker(binary, workDir string, workers int, timeout time.Duration, services map[string]string) *DeepChecker {
	if workers < 1 {
		workers = 1
	}
	if workers > 8 {
		workers = 8
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &DeepChecker{Binary: binary, WorkDir: workDir, Workers: workers, Timeout: timeout, ActiveServices: services}
}

type deepWorker struct {
	lx        *lxruntime.LXD
	mixedPort int
	workDir   string
}

func (d *DeepChecker) newWorker(ctx context.Context, index int) (*deepWorker, error) {
	mixed, err := lxruntime.FreeTCPPort()
	if err != nil {
		return nil, err
	}
	controlPort, err := lxruntime.FreeTCPPort()
	if err != nil {
		return nil, err
	}
	state := fmt.Sprintf("%s/worker-%d", d.WorkDir, index)
	lx := lxruntime.NewLXD(d.Binary, state)
	lx.ControlPort = controlPort
	if err := lx.Start(ctx, nil); err != nil {
		return nil, err
	}
	return &deepWorker{lx: lx, mixedPort: mixed, workDir: state}, nil
}

func (w *deepWorker) close() {
	if w != nil && w.lx != nil {
		_ = w.lx.Stop()
	}
}

func (w *deepWorker) check(ctx context.Context, node model.NodeInfo, timeout time.Duration, services map[string]string) model.FinalResult {
	r := model.FinalResult{Node: node, SuccessServices: map[string]bool{}, ServiceLatency: map[string]time.Duration{}, ServiceErrors: map[string]string{}}
	cfg, err := MarshalServiceConfig(node, w.mixedPort)
	if err != nil {
		r.Error = err
		return r
	}
	applyCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := w.lx.Apply(applyCtx, cfg); err != nil {
		r.Error = err
		return r
	}
	total := 0
	success := 0
	var latency time.Duration
	for name, target := range services {
		if applyCtx.Err() != nil {
			break
		}
		ok, lat := checkService(applyCtx, newProxyClient(fmt.Sprintf("http://127.0.0.1:%d", w.mixedPort), minDuration(5*time.Second, timeout)), target)
		total++
		if ok {
			success++
			latency += lat
			r.SuccessServices[name] = true
			r.ServiceLatency[name] = lat
		} else {
			r.ServiceErrors[name] = fmt.Sprintf("probe failed after %s", lat)
		}
	}
	if total == 0 {
		r.Error = errors.New("no active services configured")
		return r
	}
	for name := range services {
		if _, ok := r.SuccessServices[name]; !ok {
			r.SuccessServices[name] = false
		}
	}
	r.SuccessRate = float64(success) / float64(total)
	if success > 0 {
		r.Alive = true
		r.Latency = latency / time.Duration(success)
		r.StabilityScore = int(r.SuccessRate * 100)
		r.Score = coreranking.CalculateScore(r.Latency) + r.StabilityScore + len(r.SuccessServices)*50
		return r
	}
	r.Error = errors.New("all services failed")
	return r
}

func newProxyClient(proxyAddr string, timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	proxyURL, err := url.Parse(proxyAddr)
	if err != nil {
		return &http.Client{Timeout: timeout}
	}
	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		DialContext: (&net.Dialer{
			Timeout: 5 * time.Second,
		}).DialContext,
	}
	return &http.Client{Transport: transport, Timeout: timeout}
}

func checkService(ctx context.Context, client *http.Client, target string) (bool, time.Duration) {
	start := time.Now()
	if client == nil || target == "" {
		return false, 0
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return false, 0
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, time.Since(start)
	}
	defer resp.Body.Close()
	return resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices, time.Since(start)
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func (d *DeepChecker) StreamDeepCheck(ctx context.Context, in <-chan model.NodeInfo, pm *progress.Manager, gate ...control.Gate) <-chan model.FinalResult {
	out := make(chan model.FinalResult, d.Workers)
	var g control.Gate
	if len(gate) > 0 {
		g = gate[0]
	}
	if pm != nil {
		pm.SetWorkers("DEEP CHECK", d.Workers)
		pm.StartStage("DEEP CHECK")
	}
	var wg sync.WaitGroup
	for i := 0; i < d.Workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			w, err := d.newWorker(ctx, index)
			if err != nil {
				if logger.Log != nil {
					logger.Log.Error("deep worker start failed", zap.Int("worker", index), zap.Error(err))
				}
				return
			}
			defer w.close()
			for {
				select {
				case <-ctx.Done():
					return
				case node, ok := <-in:
					if !ok {
						return
					}
					if g != nil && g.Wait(ctx) != nil {
						return
					}
					r := w.check(ctx, node, d.Timeout, d.ActiveServices)
					if pm != nil {
						if r.Alive {
							pm.Success("DEEP CHECK")
							pm.Produce("DEEP CHECK", 1)
						} else {
							pm.Fail("DEEP CHECK")
						}
					}
					select {
					case <-ctx.Done():
						return
					case out <- r:
					}
				}
			}
		}(i)
	}
	go func() {
		wg.Wait()
		if pm != nil {
			pm.Complete("DEEP CHECK")
		}
		close(out)
	}()
	return out
}
