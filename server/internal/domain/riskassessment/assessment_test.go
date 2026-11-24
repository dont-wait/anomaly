package riskassessment

import (
	"math"
	"testing"
	"time"
)

func TestModelResponseValidate(t *testing.T) {
	valid := ModelResponse{
		SchemaVersion: 1, EventID: "event-1", OccurredAt: time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC), ScoredAt: time.Date(2026, 10, 28, 10, 0, 1, 0, time.UTC),
		TransactionID: "TX001", RiskScore: 0.9821, PredictedLabel: 1,
		Threshold: 0.9517361521720886, ModelVersion: "model-v1", FeatureVersion: "features-v1",
	}
	valid.Signature, _ = Sign(valid, "test-key")
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid response rejected: %v", err)
	}

	tests := []struct {
		name string
		edit func(*ModelResponse)
	}{
		{name: "missing transaction", edit: func(r *ModelResponse) { r.TransactionID = "" }},
		{name: "invalid score", edit: func(r *ModelResponse) { r.RiskScore = 1.1 }},
		{name: "nonfinite score", edit: func(r *ModelResponse) { r.RiskScore = math.NaN() }},
		{name: "nonfinite threshold", edit: func(r *ModelResponse) { r.Threshold = math.Inf(1) }},
		{name: "invalid label", edit: func(r *ModelResponse) { r.PredictedLabel = 2 }},
		{name: "inconsistent label", edit: func(r *ModelResponse) { r.PredictedLabel = 0 }},
		{name: "missing model version", edit: func(r *ModelResponse) { r.ModelVersion = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			test.edit(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestNewAssessmentKeepsDecisionReasonAndFactors(t *testing.T) {
	z := 4.2
	response := ModelResponse{
		SchemaVersion: 1, EventID: "e", TransactionID: "t", RiskScore: 0.9, PredictedLabel: 1,
		Threshold: 0.5, ModelVersion: "m", FeatureVersion: "f", Signature: "s",
		OccurredAt: time.Now(), ScoredAt: time.Now(),
		TopFactors: []Factor{{Feature: "log_amount", ZScore: &z}},
	}
	got, err := NewAssessment(response, time.Time{}, "topic", 0, 1, "id")
	if err != nil {
		t.Fatal(err)
	}
	if got.Decision != "BLOCK" || got.DecisionReason == "" || len(got.TopFactors) != 1 {
		t.Fatalf("unexpected assessment: %+v", got)
	}
}
