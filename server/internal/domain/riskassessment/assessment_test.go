package riskassessment

import (
	"math"
	"reflect"
	"strings"
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
	z, flag := 4.2, 1.0
	factors := []Factor{{Feature: "log_amount", ZScore: &z}, {Feature: "new_receiver", Value: &flag}}
	response := ModelResponse{
		SchemaVersion: 1, EventID: "e", TransactionID: "t", RiskScore: 0.9, PredictedLabel: 1,
		Threshold: 0.5, ModelVersion: "m", FeatureVersion: "f", Signature: "s",
		OccurredAt: time.Now(), ScoredAt: time.Now(), TopFactors: factors,
	}
	got, err := NewAssessment(response, time.Time{}, "topic", 0, 1, "id")
	if err != nil {
		t.Fatal(err)
	}
	if got.Decision != "BLOCK" {
		t.Fatalf("decision = %q, want BLOCK", got.Decision)
	}
	if want := DecisionReasonForScore(0.9); got.DecisionReason != want || !strings.Contains(want, "0.9") || !strings.Contains(want, ">") {
		t.Fatalf("decision reason = %q", got.DecisionReason)
	}
	if len(got.TopFactors) != 2 || !reflect.DeepEqual(got.TopFactors, factors) {
		t.Fatalf("factors changed: %+v", got.TopFactors)
	}
}

func TestDecisionReasonForScoreStatesTheComparison(t *testing.T) {
	if got := DecisionReasonForScore(RiskBlockScore + 0.01); !strings.Contains(got, ">") {
		t.Fatalf("score above threshold: %q", got)
	}
	if got := DecisionReasonForScore(RiskBlockScore); !strings.Contains(got, "<=") {
		t.Fatalf("score at threshold: %q", got)
	}
}

func TestModelResponseValidatesTopFactors(t *testing.T) {
	z, v := 1.5, 1.0
	nan := math.NaN()
	base := ModelResponse{
		SchemaVersion: 1, EventID: "e", TransactionID: "t", RiskScore: 0.1, PredictedLabel: 0,
		Threshold: 0.5, ModelVersion: "m", FeatureVersion: "f", Signature: "s",
		OccurredAt: time.Now(), ScoredAt: time.Now(),
	}
	tooMany := make([]Factor, MaxTopFactors+1)
	for i := range tooMany {
		tooMany[i] = Factor{Feature: "f", ZScore: &z}
	}
	cases := map[string][]Factor{
		"empty feature":    {{Feature: " ", ZScore: &z}},
		"neither number":   {{Feature: "log_amount"}},
		"both numbers":     {{Feature: "log_amount", ZScore: &z, Value: &v}},
		"non finite":       {{Feature: "log_amount", ZScore: &nan}},
		"too many":         tooMany,
		"feature too long": {{Feature: strings.Repeat("x", 65), ZScore: &z}},
	}
	for name, factors := range cases {
		candidate := base
		candidate.TopFactors = factors
		if err := candidate.Validate(); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
	ok := base
	ok.TopFactors = []Factor{{Feature: "log_amount", ZScore: &z}, {Feature: "new_receiver", Value: &v}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid factors rejected: %v", err)
	}
}
