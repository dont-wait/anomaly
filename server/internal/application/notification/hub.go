package notification

import (
	"errors"
	"sync"

	"github.com/dont-wait/anomaly/internal/domain/notification"
)

const hubClientBuffer = 32

var ErrTooManyConnections = errors.New("too many notification streams for recipient")

// Hub fans notifications out per recipient. Unlike riskstream.Hub it never
// broadcasts: a subscriber only receives its own recipient's notifications.
type Hub struct {
	mu         sync.Mutex
	clients    map[string]map[uint64]chan notification.Notification
	nextID     uint64
	maxPerUser int
}

func NewHub(maxConnPerRecipient int) *Hub {
	if maxConnPerRecipient <= 0 {
		maxConnPerRecipient = 5
	}
	return &Hub{clients: map[string]map[uint64]chan notification.Notification{}, maxPerUser: maxConnPerRecipient}
}

func (h *Hub) Subscribe(recipientKey string) (<-chan notification.Notification, func(), error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	group := h.clients[recipientKey]
	if len(group) >= h.maxPerUser {
		return nil, nil, ErrTooManyConnections
	}
	if group == nil {
		group = map[uint64]chan notification.Notification{}
		h.clients[recipientKey] = group
	}
	h.nextID++
	id := h.nextID
	channel := make(chan notification.Notification, hubClientBuffer)
	group[id] = channel

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.removeLocked(recipientKey, id)
		})
	}
	return channel, cancel, nil
}

func (h *Hub) removeLocked(recipientKey string, id uint64) {
	group := h.clients[recipientKey]
	if channel, ok := group[id]; ok {
		delete(group, id)
		close(channel)
	}
	if len(group) == 0 {
		delete(h.clients, recipientKey)
	}
}

// Publish never blocks. A client whose buffer is full is closed so it
// reconnects and replays from its cursor instead of silently missing items.
func (h *Hub) Publish(n notification.Notification) {
	key := notification.RecipientKey(n.Audience, n.RecipientID)
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, client := range h.clients[key] {
		select {
		case client <- n:
		default:
			h.removeLocked(key, id)
		}
	}
}

// DisconnectAll đóng mọi stream đang mở nhưng hub vẫn nhận subscriber mới.
// Dùng khi change stream bị gap (vd mất history): client sẽ reconnect và
// replay từ cursor nên không bỏ lỡ thông báo phát sinh trong khoảng gap.
func (h *Hub) DisconnectAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for key, group := range h.clients {
		for id := range group {
			h.removeLocked(key, id)
		}
	}
}

// Close đóng mọi stream khi API shutdown.
func (h *Hub) Close() { h.DisconnectAll() }

// Subscribers reports open streams for a recipient.
func (h *Hub) Subscribers(recipientKey string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients[recipientKey])
}
