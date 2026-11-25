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
		"reason too long":  {{Feature: "log_amount", ZScore: &z, Reason: strings.Repeat("ọ", MaxFactorReasonLen+1)}},
	}
	for name, factors := range cases {
		candidate := base
		candidate.TopFactors = factors
		if err := candidate.Validate(); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
	ok := base
	ok.TopFactors = []Factor{
		{Feature: "log_amount", ZScore: &z, Reason: "Số tiền chuyển lớn hơn bình thường"},
		{Feature: "new_receiver", Value: &v},
	}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid factors rejected: %v", err)
	}
}

func validScenario() Scenario {
	return Scenario{
		Code: "ACCOUNT_TAKEOVER", Severity: "high", Title: "Nghi chiếm đoạt tài khoản",
		Reason:   "Thiết bị mới + người nhận mới + số tiền gấp 9.0 lần mức trung vị 30 ngày",
		Evidence: []string{"new_device", "new_receiver"},
	}
}

func TestModelResponseValidatesScenarios(t *testing.T) {
	base := ModelResponse{
		SchemaVersion: 1, EventID: "e", TransactionID: "t", RiskScore: 0.1, PredictedLabel: 0,
		Threshold: 0.5, ModelVersion: "m", FeatureVersion: "f", Signature: "s",
		OccurredAt: time.Now(), ScoredAt: time.Now(),
	}
	mutate := func(f func(*Scenario)) []Scenario {
		s := validScenario()
		f(&s)
		return []Scenario{s}
	}
	tooMany := make([]Scenario, MaxScenarios+1)
	for i := range tooMany {
		tooMany[i] = validScenario()
	}
	cases := map[string][]Scenario{
		"lowercase code":  mutate(func(s *Scenario) { s.Code = "account_takeover" }),
		"empty code":      mutate(func(s *Scenario) { s.Code = "" }),
		"code too long":   mutate(func(s *Scenario) { s.Code = "A" + strings.Repeat("B", 60) }),
		"bad severity":    mutate(func(s *Scenario) { s.Severity = "critical" }),
		"empty title":     mutate(func(s *Scenario) { s.Title = " " }),
		"title too long":  mutate(func(s *Scenario) { s.Title = strings.Repeat("ọ", MaxScenarioTitleLen+1) }),
		"empty reason":    mutate(func(s *Scenario) { s.Reason = "" }),
		"reason too long": mutate(func(s *Scenario) { s.Reason = strings.Repeat("ọ", MaxScenarioReasonLen+1) }),
		"too much evidence": mutate(func(s *Scenario) {
			s.Evidence = make([]string, MaxScenarioEvidence+1)
			for i := range s.Evidence {
				s.Evidence[i] = "f"
			}
		}),
		"evidence name too long": mutate(func(s *Scenario) { s.Evidence = []string{strings.Repeat("x", 65)} }),
		"too many scenarios":     tooMany,
	}
	for name, scenarios := range cases {
		candidate := base
		candidate.Scenarios = scenarios
		if err := candidate.Validate(); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
	ok := base
	ok.Scenarios = []Scenario{validScenario()}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid scenarios rejected: %v", err)
	}
}

func TestNewAssessmentKeepsScenarios(t *testing.T) {
	scenarios := []Scenario{validScenario()}
	response := ModelResponse{
		SchemaVersion: 1, EventID: "e", TransactionID: "t", RiskScore: 0.9, PredictedLabel: 1,
		Threshold: 0.5, ModelVersion: "m", FeatureVersion: "f", Signature: "s",
		OccurredAt: time.Now(), ScoredAt: time.Now(), Scenarios: scenarios,
	}
	got, err := NewAssessment(response, time.Time{}, "topic", 0, 1, "id")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Scenarios, scenarios) {
		t.Fatalf("scenarios changed: %+v", got.Scenarios)
	}
}
