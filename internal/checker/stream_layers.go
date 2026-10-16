package checker

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/core/control"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/model"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/progress"
)

func StreamParseNodes(ctx context.Context, in <-chan string, workers int, pm *progress.Manager, gate ...control.Gate) <-chan model.NodeInfo {
	if workers < 1 {
		workers = 1
	}
	out := make(chan model.NodeInfo, 204800)
	var wg sync.WaitGroup
	var g control.Gate
	if len(gate) > 0 {
		g = gate[0]
	}
	if pm != nil {
		pm.SetWorkers("PARSER", workers)
		pm.StartStage("PARSER")
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case line, ok := <-in:
					if !ok {
						return
					}
					if g != nil && g.Wait(ctx) != nil {
						return
					}
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
					select {
					case <-ctx.Done():
						return
					case out <- node:
						if pm != nil {
							pm.Success("PARSER")
							pm.Produce("PARSER", 1)
						}
					}
				}
			}
		}()
	}
	go func() {
		wg.Wait()
		if pm != nil {
			pm.Complete("PARSER")
		}
		close(out)
	}()
	return out
}

// StreamTCPCheck is the single cheap network gate. Successful nodes are forwarded
// immediately to the bounded sing-box-lx Deep Check worker pool.
func StreamTCPCheck(ctx context.Context, in <-chan model.NodeInfo, timeout time.Duration, workers int, pm *progress.Manager, gate ...control.Gate) <-chan model.NodeInfo {
	if workers < 1 {
		workers = 1
	}
	out := make(chan model.NodeInfo, 204800)
	var wg sync.WaitGroup
	var g control.Gate
	if len(gate) > 0 {
		g = gate[0]
	}
	if pm != nil {
		pm.SetWorkers("TCP CHECK", workers)
		pm.StartStage("TCP CHECK")
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
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
					latency, err := TCPCheck(node, timeout)
					if err != nil {
						if pm != nil {
							pm.Fail("TCP CHECK")
						}
						continue
					}
					node.TCPDelay = latency
					if pm != nil {
						pm.Success("TCP CHECK")
						pm.Produce("TCP CHECK", 1)
					}
					select {
					case <-ctx.Done():
						return
					case out <- node:
					}
				}
			}
		}()
	}
	go func() {
		wg.Wait()
		if pm != nil {
			pm.Complete("TCP CHECK")
		}
		close(out)
	}()
	return out
}
