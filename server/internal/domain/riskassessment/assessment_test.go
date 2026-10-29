package riskassessment

import (
	"math"
	"testing"
)

func TestModelResponseValidate(t *testing.T) {
	valid := ModelResponse{
		TransactionID: "TX001", RiskScore: 0.9821, PredictedLabel: 1,
		Threshold: 0.9517361521720886, ModelVersion: "model-v1", FeatureVersion: "features-v1",
	}
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
