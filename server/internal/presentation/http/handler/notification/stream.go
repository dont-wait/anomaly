package notification

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	notificationapp "github.com/dont-wait/anomaly/internal/application/notification"
	notificationdomain "github.com/dont-wait/anomaly/internal/domain/notification"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
	"github.com/dont-wait/anomaly/internal/presentation/http/sse"
)

const replayPageSize = 200

// Stream replays notifications after the client's cursor, then pushes live
// ones from the hub. Event ids are cursors, so a reconnecting client resumes
// with Last-Event-ID (or ?cursor=) without gaps.
func (h *Handler) stream(w http.ResponseWriter, r *http.Request) {
	recipient, ok := recipientFrom(r)
	if !ok {
		h.fail(w, errUnauthorized)
		return
	}
	claims, _ := middleware.ClaimsFromContext(r.Context())
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}
	rawCursor := r.URL.Query().Get("cursor")
	if rawCursor == "" {
		rawCursor = r.Header.Get("Last-Event-ID")
	}
	after, err := notificationdomain.DecodeCursor(rawCursor)
	if err != nil {
		h.fail(w, err)
		return
	}
	// Subscribe before replay so nothing inserted during replay is missed;
	// duplicates are dropped by cursor comparison below.
	updates, cancel, err := h.hub.Subscribe(recipient.Key())
	if err != nil {
		h.fail(w, err)
		return
	}
	defer cancel()

	sse.SetHeaders(w)
	if err := sse.WriteComment(w, flusher, "connected"); err != nil {
		return
	}
	if !h.writeUnreadCount(w, flusher, r.Context(), recipient) {
		return
	}

	if after != nil {
		for {
			queryCtx, queryCancel := context.WithTimeout(r.Context(), 5*time.Second)
			items, err := h.queries.ListAfter(queryCtx, recipient, *after, replayPageSize)
			queryCancel()
			if err != nil {
				h.log.Error().Err(err).Msg("notification stream replay failed")
				return
			}
			for _, item := range items {
				if !writeNotification(w, flusher, item) {
					return
				}
				next := notificationdomain.CursorOf(item)
				after = &next
			}
			if len(items) < replayPageSize {
				break
			}
		}
	}

	ticker := time.NewTicker(sse.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case item, open := <-updates:
			if !open {
				return // hub closed a slow client: reconnect and replay
			}
			if after != nil && !isAfter(item, *after) {
				continue
			}
			if !writeNotification(w, flusher, item) {
				return
			}
			next := notificationdomain.CursorOf(item)
			after = &next
			if !h.writeUnreadCount(w, flusher, r.Context(), recipient) {
				return
			}
		case <-ticker.C:
			if claims != nil && !claims.ExpiresAt.IsZero() && !time.Now().Before(claims.ExpiresAt) {
				return
			}
			if err := sse.WriteComment(w, flusher, "ping"); err != nil {
				return
			}
		}
	}
}

func isAfter(n notificationdomain.Notification, cursor notificationdomain.Cursor) bool {
	if n.CreatedAt.Equal(cursor.CreatedAt) {
		return n.ID > cursor.ID
	}
	return n.CreatedAt.After(cursor.CreatedAt)
}

func writeNotification(w http.ResponseWriter, flusher http.Flusher, item notificationdomain.Notification) bool {
	payload, err := json.Marshal(toDTO(item))
	if err != nil {
		return false
	}
	return sse.WriteEvent(w, flusher, notificationdomain.CursorOf(item).Encode(), "notification", payload) == nil
}

func (h *Handler) writeUnreadCount(w http.ResponseWriter, flusher http.Flusher, ctx context.Context, recipient notificationapp.Recipient) bool {
	payload, err := h.countUnread(ctx, recipient)
	if err != nil {
		h.log.Error().Err(err).Msg("notification unread count failed")
		return false
	}
	return sse.WriteEvent(w, flusher, "", "unread-count", payload) == nil
}
