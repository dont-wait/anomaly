package transaction

import (
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/risk"
	"github.com/google/uuid"
)

func TestFailedTransferEventRequiresRiskAssessment(t *testing.T) {
	eventID := uuid.NewString()
	txID := uuid.NewString()
	event := Event{
		ID: eventID, TransactionID: txID, Type: Failed, SchemaVersion: SchemaVersion,
		Sequence: 2, OccurredAt: time.Now().UTC(),
		Payload: Payload{
			SourceAccountID: "source", DestinationAccountID: "destination", Amount: 1000,
			Currency: "VND", Channel: "web", Status: "failed", FailureReason: "anomalous_transfer",
		},
	}
	if err := event.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing risk assessment error")
	}
	event.Payload.RiskAssessment = &risk.Assessment{
		IsAnomalous: true, RiskScore: 0.97, ModelVersion: "model-v1", EvaluatedAt: time.Now().UTC(),
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want valid failed event", err)
	}
}

func TestCompletedTransferEventAllowsRiskAssessment(t *testing.T) {
	sourceBefore, sourceAfter := int64(10000), int64(9000)
	destinationBefore, destinationAfter := int64(1000), int64(2000)
	event := Event{
		ID: uuid.NewString(), TransactionID: uuid.NewString(), Type: Completed, SchemaVersion: SchemaVersion,
		Sequence: 2, OccurredAt: time.Now().UTC(),
		Payload: Payload{
			SourceAccountID: "source", DestinationAccountID: "destination", Amount: 1000,
			Currency: "VND", Channel: "web", Status: "success",
			SourceBalanceBefore: &sourceBefore, SourceBalanceAfter: &sourceAfter,
			DestinationBalanceBefore: &destinationBefore, DestinationBalanceAfter: &destinationAfter,
			RiskAssessment: &risk.Assessment{IsAnomalous: false, RiskScore: 0.08, ModelVersion: "model-v1", EvaluatedAt: time.Now().UTC()},
		},
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want valid completed event", err)
	}
}
