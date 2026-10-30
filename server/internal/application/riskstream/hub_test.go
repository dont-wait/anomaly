package riskstream

import (
	"testing"

	"github.com/dont-wait/anomaly/internal/domain/riskassessment"
)

func TestHubClosesSlowClientForCursorReplay(t *testing.T) {
	hub := NewHub()
	updates, _ := hub.Subscribe()
	for index := 0; index < 33; index++ {
		hub.Publish(Event{ID: string(rune('a' + index)), Assessment: riskassessment.Assessment{ID: "assessment"}})
	}

	for index := 0; index < 32; index++ {
		if _, open := <-updates; !open {
			t.Fatalf("client closed before buffer was full at event %d", index)
		}
	}
	if _, open := <-updates; open {
		t.Fatal("slow client should be closed so it can reconnect and replay")
	}
}
