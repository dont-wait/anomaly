package risk

import (
	"testing"
	"time"
)

func TestDecisionValidationSupportsPendingAndTerminalResults(t *testing.T) {
	pending := Decision{TransferID: "transfer-1", SourceEventID: "event-1", Decision: DecisionPending}
	if err := pending.Validate(); err != nil {
		t.Fatalf("pending decision validation = %v", err)
	}

	assessment := Assessment{
		IsAnomalous:  false,
		RiskScore:    0.08,
		ModelVersion: "model-v1",
		EvaluatedAt:  time.Now().UTC(),
	}
	allowed := Decision{TransferID: "transfer-1", SourceEventID: "event-1", Decision: DecisionAllow, Assessment: assessment}
	if err := allowed.Validate(); err != nil {
		t.Fatalf("allow decision validation = %v", err)
	}

	blocked := Decision{TransferID: "transfer-1", SourceEventID: "event-1", Decision: DecisionBlock, Assessment: Assessment{
		IsAnomalous:  true,
		RiskScore:    0.97,
		ModelVersion: "model-v1",
		EvaluatedAt:  assessment.EvaluatedAt,
	}}
	if err := blocked.Validate(); err != nil {
		t.Fatalf("block decision validation = %v", err)
	}
}

func TestDecisionValidationRejectsMismatchedAssessmentDecision(t *testing.T) {
	decision := Decision{
		TransferID:    "transfer-1",
		SourceEventID: "event-1",
		Decision:      DecisionAllow,
		Assessment: Assessment{
			Decision:     DecisionBlock,
			IsAnomalous:  false,
			RiskScore:    0.08,
			ModelVersion: "model-v1",
			EvaluatedAt:  time.Now().UTC(),
		},
	}
	if err := decision.Validate(); err == nil {
		t.Fatal("mismatched assessment decision validation = nil")
	}
}
