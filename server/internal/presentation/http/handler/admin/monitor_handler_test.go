package admin

import (
	"context"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/riskassessment"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
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

type transactionContextLookupFake struct {
	calls    int
	contexts map[string]mongorepo.RiskTransactionContext
}

func (f *transactionContextLookupFake) FindRiskTransactionContexts(_ context.Context, _ []string) (map[string]mongorepo.RiskTransactionContext, error) {
	f.calls++
	return f.contexts, nil
}

func TestEnrichAssessmentsUsesOneBatchLookup(t *testing.T) {
	lookup := &transactionContextLookupFake{contexts: map[string]mongorepo.RiskTransactionContext{
		"TX001": {TransactionID: "TX001", SourceAccountNo: "ACC001", Amount: 50000000, Currency: "VND"},
		"TX002": {TransactionID: "TX002", SourceAccountNo: "ACC002", Amount: 25000000, Currency: "VND"},
	}}
	handler := NewMonitorHandler(nil, nil, lookup)
	items, err := handler.enrichAssessments(context.Background(), []riskassessment.Assessment{
		{ID: "a1", TransactionID: "TX001"},
		{ID: "a2", TransactionID: "TX002"},
	})
	if err != nil {
		t.Fatalf("enrich assessments: %v", err)
	}
	if lookup.calls != 1 {
		t.Fatalf("lookup calls = %d, want 1", lookup.calls)
	}
	if items[0].Transaction == nil || items[0].Transaction.SourceAccountNo != "ACC001" {
		t.Fatalf("first transaction context = %+v", items[0].Transaction)
	}
}
