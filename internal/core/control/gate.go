package control

import (
	"context"
	"sync"
)

type Gate interface{ Wait(context.Context) error }

type PauseGate struct {
	mu     sync.RWMutex
	paused bool
	wake   chan struct{}
}

func NewPauseGate() *PauseGate { return &PauseGate{wake: make(chan struct{})} }
func (g *PauseGate) Pause() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.paused {
		return
	}
	g.paused = true
}
func (g *PauseGate) Resume() {
	g.mu.Lock()
	if !g.paused {
		g.mu.Unlock()
		return
	}
	g.paused = false
	close(g.wake)
	g.wake = make(chan struct{})
	g.mu.Unlock()
}
func (g *PauseGate) IsPaused() bool { g.mu.RLock(); defer g.mu.RUnlock(); return g.paused }
func (g *PauseGate) Wait(ctx context.Context) error {
	for {
		g.mu.RLock()
		paused := g.paused
		wake := g.wake
		g.mu.RUnlock()
		if !paused {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-wake:
		}
	}
}
