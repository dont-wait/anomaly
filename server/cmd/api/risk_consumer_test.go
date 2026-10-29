package main

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/dont-wait/anomaly/internal/domain/risk"
	"github.com/dont-wait/anomaly/internal/domain/riskassessment"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	kafkago "github.com/segmentio/kafka-go"
)

const testRiskSigningKey = "test-risk-signing-key"

func TestDecodeModelResponse(t *testing.T) {
	payload := signedModelPayload(t, []byte(`{"schema_version":1,"event_id":"event-1","transaction_id":"TX001","occurred_at":"2026-10-28T10:00:00Z","scored_at":"2026-10-28T10:00:01Z","risk_score":0.9821,"predicted_label":1,"threshold":0.9517361521720886,"model_version":"gatv2-device-control-v1","feature_version":"schema-3-0b9efd44d793"}`))
	response, err := decodeModelResponse(payload, testRiskSigningKey)
	if err != nil {
		t.Fatalf("valid model response rejected: %v", err)
	}
	if response.TransactionID != "TX001" || response.PredictedLabel != 1 || response.Signature == "" {
		t.Fatalf("unexpected model response: %+v", response)
	}
}

func TestDecodeModelResponseRejectsInvalidSignature(t *testing.T) {
	payload := signedModelPayload(t, []byte(`{"schema_version":1,"event_id":"event-1","transaction_id":"TX001","occurred_at":"2026-10-28T10:00:00Z","scored_at":"2026-10-28T10:00:01Z","risk_score":0.1,"predicted_label":0,"threshold":0.9,"model_version":"model-v1","feature_version":"features-v1"}`))
	if _, err := decodeModelResponse(payload, "wrong-key"); err == nil {
		t.Fatal("model response signed with another key was accepted")
	}
}

func TestDecodeModelResponseRejectsUnknownAndTrailingFields(t *testing.T) {
	base := `{"schema_version":1,"event_id":"event-1","transaction_id":"TX001","occurred_at":"2026-10-28T10:00:00Z","scored_at":"2026-10-28T10:00:01Z","risk_score":0.1,"predicted_label":0,"threshold":0.9,"model_version":"model-v1","feature_version":"features-v1"}`
	unknown := []byte(`{"schema_version":1,"event_id":"event-1","transaction_id":"TX001","occurred_at":"2026-10-28T10:00:00Z","scored_at":"2026-10-28T10:00:01Z","risk_score":0.1,"predicted_label":0,"threshold":0.9,"model_version":"model-v1","feature_version":"features-v1","extra":true}`)
	if _, err := decodeModelResponse(unknown, testRiskSigningKey); err == nil {
		t.Fatal("unknown model response field was accepted")
	}
	trailing := append(signedModelPayload(t, []byte(base)), []byte(" {}")...)
	if _, err := decodeModelResponse(trailing, testRiskSigningKey); err == nil {
		t.Fatal("trailing model response data was accepted")
	}
}

func signedModelPayload(t *testing.T, payload []byte) []byte {
	t.Helper()
	var response riskassessment.ModelResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatal(err)
	}
	var err error
	response.Signature, err = riskassessment.Sign(response, testRiskSigningKey)
	if err != nil {
		t.Fatal(err)
	}
	result, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestDecodeModelResponseRequiresNumericFields(t *testing.T) {
	for _, field := range []string{"risk_score", "predicted_label", "threshold"} {
		for _, mode := range []string{"missing", "null"} {
			t.Run(field+"/"+mode, func(t *testing.T) {
				input := map[string]any{
					"transaction_id": "TX001", "risk_score": 0.1,
					"predicted_label": 0, "threshold": 0.9,
					"model_version": "v1", "feature_version": "f1",
				}
				if mode == "missing" {
					delete(input, field)
				} else {
					input[field] = nil
				}
				payload, err := json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := decodeModelResponse(payload, testRiskSigningKey); err == nil || !strings.Contains(err.Error(), "required and cannot be null") {
					t.Fatalf("expected missing-field rejection for %s %s, got %v", mode, field, err)
				}
			})
		}
	}
}

func TestDecodeModelResponseAcceptsExplicitZeroValues(t *testing.T) {
	for _, payload := range []string{
		`{"transaction_id":"TX001","risk_score":0,"predicted_label":0,"threshold":0.9,"model_version":"v1","feature_version":"f1"}`,
		`{"transaction_id":"TX001","risk_score":0,"predicted_label":1,"threshold":0,"model_version":"v1","feature_version":"f1"}`,
	} {
		if _, err := decodeModelResponse(signedModelResponse(t, []byte(payload)), testRiskSigningKey); err != nil {
			t.Fatalf("valid zero values rejected: %v", err)
		}
	}
}
