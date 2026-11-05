// Package sse holds the shared Server-Sent Events plumbing used by the admin
// monitor and the notification stream.
package sse

import (
	"fmt"
	"net/http"
	"time"
)

const (
	WriteTimeout      = 30 * time.Second
	HeartbeatInterval = 15 * time.Second
)

// SetHeaders sets the streaming headers that keep proxies from caching or
// buffering the response.
func SetHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
}

// SetWriteDeadline bounds each write so a stalled client cannot pin the
// connection forever. Call it before every write.
func SetWriteDeadline(w http.ResponseWriter) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(WriteTimeout))
}

// WriteEvent writes one event and flushes it. id may be empty.
func WriteEvent(w http.ResponseWriter, flusher http.Flusher, id, event string, data []byte) error {
	SetWriteDeadline(w)
	if id != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", id); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

// WriteComment writes an SSE comment line such as a heartbeat.
func WriteComment(w http.ResponseWriter, flusher http.Flusher, comment string) error {
	SetWriteDeadline(w)
	if _, err := fmt.Fprintf(w, ": %s\n\n", comment); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}
