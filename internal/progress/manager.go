package progress

import (
	"sync"
	"time"
)

const UnknownTotal = -1

type State string

const (
	Waiting     State = "WAITING"
	Running     State = "RUNNING"
	Paused      State = "PAUSED"
	Stalled     State = "STALLED"
	Completed   State = "COMPLETED"
	FailedState State = "ERROR"
)

type Stage struct {
	Name       string    `json:"name"`
	Total      int       `json:"total"`
	Processed  int       `json:"processed"`
	Success    int       `json:"success"`
	Failed     int       `json:"failed"`
	Produced   int       `json:"produced"`
	Alive      int       `json:"alive"`
	Workers    int       `json:"workers"`
	Active     bool      `json:"active"`
	Done       bool      `json:"done"`
	State      State     `json:"state"`
	StartedAt  time.Time `json:"-"`
	FinishedAt time.Time `json:"-"`
}
type Manager struct {
	mu     sync.RWMutex
	stages []*Stage
	index  map[string]*Stage
	links  map[string][]string
}

func NewManager() *Manager {
	return &Manager{stages: make([]*Stage, 0), index: make(map[string]*Stage), links: make(map[string][]string)}
}
func (m *Manager) AddStage(name string, total, workers int) *Stage {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.index[name]; ok {
		s.Total = total
		s.Workers = workers
		return s
	}
	s := &Stage{Name: name, Total: total, Workers: workers, State: Waiting}
	m.stages = append(m.stages, s)
	m.index[name] = s
	return s
}
func (m *Manager) Link(from, to string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.links[from] = append(m.links[from], to)
}
func (m *Manager) StartStage(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.getOrCreateLocked(name)
	s.Processed = 0
	s.Success = 0
	s.Failed = 0
	s.Produced = 0
	s.Alive = 0
	s.Active = true
	s.Done = false
	s.State = Running
	s.StartedAt = time.Now()
	s.FinishedAt = time.Time{}
}
func (m *Manager) SetTotal(name string, total int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getOrCreateLocked(name).Total = total
}
func (m *Manager) SetWorkers(name string, w int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getOrCreateLocked(name).Workers = w
}
func (m *Manager) Add(name string, n int) {
	if n <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getOrCreateLocked(name).Processed += n
}
func (m *Manager) Success(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.getOrCreateLocked(name)
	s.Processed++
	s.Success++
}
func (m *Manager) Fail(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.getOrCreateLocked(name)
	s.Processed++
	s.Failed++
}
func (m *Manager) AddSuccess(name string, n int) {
	if n <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getOrCreateLocked(name).Success += n
}
func (m *Manager) Produce(name string, n int) {
	if n <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.getOrCreateLocked(name)
	s.Produced += n
	s.Alive += n
}
func (m *Manager) PauseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.stages {
		if s.Active {
			s.State = Paused
		}
	}
}
func (m *Manager) ResumeAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.stages {
		if s.Active {
			s.State = Running
		}
	}
}
func (m *Manager) Stall(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.getOrCreateLocked(name)
	if s.Active {
		s.State = Stalled
	}
}
func (m *Manager) Complete(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.getOrCreateLocked(name)
	if s.Total < 0 {
		s.Total = s.Processed
	}
	s.Active = false
	s.Done = true
	s.State = Completed
	s.FinishedAt = time.Now()
	for _, n := range m.links[name] {
		m.getOrCreateLocked(n).Total = s.Produced
	}
}
func (m *Manager) FailStage(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.getOrCreateLocked(name)
	if s.Total < 0 {
		s.Total = s.Processed
	}
	s.Active = false
	s.Done = true
	s.State = FailedState
	s.FinishedAt = time.Now()
}
func (m *Manager) Snapshot() []Stage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r := make([]Stage, len(m.stages))
	for i, s := range m.stages {
		r[i] = *s
	}
	return r
}
func (m *Manager) getOrCreateLocked(name string) *Stage {
	if s, ok := m.index[name]; ok {
		return s
	}
	s := &Stage{Name: name, Total: UnknownTotal, State: Waiting}
	m.stages = append(m.stages, s)
	m.index[name] = s
	return s
}
