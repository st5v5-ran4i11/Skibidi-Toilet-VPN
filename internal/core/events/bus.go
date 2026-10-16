package events

import "sync"

type Type string

const (
	Start       Type = "START"
	Progress    Type = "PROGRESS"
	StageChange Type = "STAGE_CHANGE"
	Warning     Type = "WARNING"
	Error       Type = "ERROR"
	Pause       Type = "PAUSE"
	Resume      Type = "RESUME"
	Complete    Type = "COMPLETE"
)

type Event struct {
	Type    Type   `json:"type"`
	Stage   string `json:"stage,omitempty"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}
type Bus struct {
	mu   sync.RWMutex
	next uint64
	subs map[uint64]chan Event
}

func NewBus() *Bus { return &Bus{subs: make(map[uint64]chan Event)} }
func (b *Bus) Subscribe(buffer int) (<-chan Event, func()) {
	if buffer < 1 {
		buffer = 32
	}
	ch := make(chan Event, buffer)
	b.mu.Lock()
	b.next++
	id := b.next
	b.subs[id] = ch
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		if c, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(c)
		}
		b.mu.Unlock()
	}
}
func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
}
