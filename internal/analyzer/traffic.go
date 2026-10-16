package analyzer

import (
	"sync"
	"time"
)

type Mode string

const (
	Off        Mode = "OFF"
	Passive    Mode = "PASSIVE"
	Adaptive   Mode = "ADAPTIVE"
	Aggressive Mode = "AGGRESSIVE"
)

type Event struct {
	Timestamp time.Time     `json:"timestamp"`
	Service   string        `json:"service"`
	Domain    string        `json:"domain"`
	TargetIP  string        `json:"target_ip,omitempty"`
	Outbound  string        `json:"outbound,omitempty"`
	NodeID    string        `json:"node,omitempty"`
	Latency   time.Duration `json:"latency"`
	Success   bool          `json:"success"`
	Timeout   bool          `json:"timeout"`
	Error     string        `json:"error,omitempty"`
}
type Analyzer struct {
	mu     sync.RWMutex
	mode   Mode
	events []Event
	limit  int
}

func New(mode Mode, limit int) *Analyzer {
	if limit <= 0 {
		limit = 5000
	}
	return &Analyzer{mode: mode, limit: limit}
}
func (a *Analyzer) Record(e Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.mode == Off {
		return
	}
	e.Timestamp = time.Now()
	a.events = append(a.events, e)
	if len(a.events) > a.limit {
		a.events = a.events[len(a.events)-a.limit:]
	}
}
func (a *Analyzer) Snapshot() []Event {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return append([]Event(nil), a.events...)
}
func (a *Analyzer) Mode() Mode { a.mu.RLock(); defer a.mu.RUnlock(); return a.mode }
