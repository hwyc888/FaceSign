package realtime

import (
	"encoding/json"
	"sync"
)

type Event struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

type Hub struct {
	mu          sync.RWMutex
	subscribers map[chan []byte]struct{}
}

func NewHub() *Hub {
	return &Hub{subscribers: make(map[chan []byte]struct{})}
}

func (h *Hub) Subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, 8)
	h.mu.Lock()
	h.subscribers[ch] = struct{}{}
	h.mu.Unlock()

	cancel := func() {
		h.mu.Lock()
		if _, ok := h.subscribers[ch]; ok {
			delete(h.subscribers, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
	return ch, cancel
}

func (h *Hub) Publish(event Event) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for subscriber := range h.subscribers {
		select {
		case subscriber <- payload:
		default:
		}
	}
}
