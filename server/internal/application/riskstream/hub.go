package riskstream

import (
	"sync"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/riskassessment"
)

type Event struct {
	ID                string
	Assessment        riskassessment.Assessment
	TransactionStatus *TransactionStatus
}

type TransactionStatus struct {
	TransactionID string     `json:"transactionId"`
	Status        string     `json:"status"`
	StatusAt      time.Time  `json:"statusAt"`
	OTPExpiresAt  *time.Time `json:"otpExpiresAt,omitempty"`
	OTPVerifiedAt *time.Time `json:"otpVerifiedAt,omitempty"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`
	FailedAt      *time.Time `json:"failedAt,omitempty"`
	CancelledAt   *time.Time `json:"cancelledAt,omitempty"`
}

type Hub struct {
	mu      sync.RWMutex
	clients map[uint64]chan Event
	nextID  uint64
}

func NewHub() *Hub {
	return &Hub{clients: make(map[uint64]chan Event)}
}

func (h *Hub) Subscribe() (<-chan Event, func()) {
	h.mu.Lock()
	h.nextID++
	id := h.nextID
	channel := make(chan Event, 32)
	h.clients[id] = channel
	h.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			if current, ok := h.clients[id]; ok {
				delete(h.clients, id)
				close(current)
			}
			h.mu.Unlock()
		})
	}
	return channel, cancel
}

// Publish is deliberately non-blocking. The snapshot/replay API remains the
// source of truth if a slow client cannot keep up with the live stream.
func (h *Hub) Publish(event Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, client := range h.clients {
		select {
		case client <- event:
		default:
			// A slow client must reconnect and replay from its last cursor. Do
			// not silently drop an event while keeping the stream open.
			delete(h.clients, id)
			close(client)
		}
	}
}

func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, client := range h.clients {
		delete(h.clients, id)
		close(client)
	}
}
