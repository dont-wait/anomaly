package admin

import (
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/riskassessment"
)

func TestCursorRoundTripAllowsWatermarkWithoutID(t *testing.T) {
	value := cursor{ReceivedAt: time.Date(2026, 10, 28, 10, 0, 0, 123000000, time.UTC)}
	encoded := encodeCursor(value)
	decoded, err := decodeCursor(encoded)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	if !decoded.ReceivedAt.Equal(value.ReceivedAt) || decoded.ID != "" {
		t.Fatalf("decoded cursor = %+v, want %+v", decoded, value)
	}
}

func TestIsAfterUsesAssessmentIDForSameMillisecond(t *testing.T) {
	after := cursor{ReceivedAt: time.UnixMilli(10), ID: "assessment-1"}
	if isAfter(assessmentAt(time.UnixMilli(10), "assessment-0"), after) {
		t.Fatal("older assessment ID should not be after cursor")
	}
	if !isAfter(assessmentAt(time.UnixMilli(10), "assessment-2"), after) {
		t.Fatal("newer assessment ID should be after cursor")
	}
}

func assessmentAt(receivedAt time.Time, id string) riskassessment.Assessment {
	return riskassessment.Assessment{ID: id, ReceivedAt: receivedAt}
}
