package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dont-wait/anomaly/internal/domain/riskassessment"
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
					"schema_version": 1, "event_id": "event-1",
					"occurred_at": "2026-10-28T10:00:00Z", "scored_at": "2026-10-28T10:00:01Z",
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
		`{"schema_version":1,"event_id":"event-1","transaction_id":"TX001","occurred_at":"2026-10-28T10:00:00Z","scored_at":"2026-10-28T10:00:01Z","risk_score":0,"predicted_label":0,"threshold":0.9,"model_version":"v1","feature_version":"f1"}`,
		`{"schema_version":1,"event_id":"event-1","transaction_id":"TX001","occurred_at":"2026-10-28T10:00:00Z","scored_at":"2026-10-28T10:00:01Z","risk_score":0,"predicted_label":1,"threshold":0,"model_version":"v1","feature_version":"f1"}`,
	} {
		if _, err := decodeModelResponse(signedModelPayload(t, []byte(payload)), testRiskSigningKey); err != nil {
			t.Fatalf("valid zero values rejected: %v", err)
		}
	}
}

// pythonRiskPayload mirrors what anomaly-service publishes: snake_case keys, with
// top_factors attached beside (not inside) the signed fields.
func pythonRiskPayload(t *testing.T, factors []any) []byte {
	t.Helper()
	base := signedModelPayload(t, []byte(`{"schema_version":1,"event_id":"event-1","transaction_id":"TX001","occurred_at":"2026-10-28T10:00:00Z","scored_at":"2026-10-28T10:00:01Z","risk_score":0.9821,"predicted_label":1,"threshold":0.9517361521720886,"model_version":"gatv2-device-control-v1","feature_version":"schema-3-0b9efd44d793"}`))
	var wire map[string]any
	if err := json.Unmarshal(base, &wire); err != nil {
		t.Fatal(err)
	}
	wire["top_factors"] = factors
	payload, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestDecodeModelResponseAcceptsPythonTopFactors(t *testing.T) {
	payload := pythonRiskPayload(t, []any{
		map[string]any{"feature": "log_amount", "z_score": 4.213},
		map[string]any{"feature": "new_receiver", "value": 1},
		map[string]any{"feature": "channel_prior_fraction", "value": 0.0123},
	})
	response, err := decodeModelResponse(payload, testRiskSigningKey)
	if err != nil {
		t.Fatalf("response with python top_factors rejected: %v", err)
	}
	if len(response.TopFactors) != 3 || response.TopFactors[0].ZScore == nil || *response.TopFactors[0].ZScore != 4.213 ||
		response.TopFactors[1].Value == nil || *response.TopFactors[1].Value != 1 {
		t.Fatalf("unexpected factors: %+v", response.TopFactors)
	}
}

func TestDecodeModelResponseRejectsCamelCaseFactorKey(t *testing.T) {
	payload := pythonRiskPayload(t, []any{map[string]any{"feature": "log_amount", "zScore": 4.2}})
	if _, err := decodeModelResponse(payload, testRiskSigningKey); err == nil {
		t.Fatal("camelCase zScore must not be accepted on the wire")
	}
}

func TestDecodeModelResponseTopFactorsAreNotSigned(t *testing.T) {
	// Documents the contract: factors are advisory, so editing them keeps the signature valid.
	payload := pythonRiskPayload(t, []any{map[string]any{"feature": "log_amount", "z_score": 1.0}})
	tampered := strings.Replace(string(payload), `"z_score":1`, `"z_score":9`, 1)
	if _, err := decodeModelResponse([]byte(tampered), testRiskSigningKey); err != nil {
		t.Fatalf("expected unsigned factors to be ignored by verification: %v", err)
	}
}

// The exact JSON shape anomaly-service publishes (top_factors with reason, and
// scenarios), including keys that are not covered by the signature.
func TestDecodeModelResponseAcceptsTraceFieldsFromMLService(t *testing.T) {
	payload := signedModelPayload(t, []byte(`{"schema_version":1,"event_id":"event-1","transaction_id":"TX001","occurred_at":"2026-10-28T10:00:00Z","scored_at":"2026-10-28T10:00:01Z","risk_score":0.9821,"predicted_label":1,"threshold":0.9517361521720886,"model_version":"gatv2-device-control-v1","feature_version":"schema-3-0b9efd44d793","top_factors":[{"feature":"log_amount_to_type_median","z_score":4.2,"reason":"Số tiền gấp 8.0 lần mức trung vị 30 ngày của các giao dịch cùng loại (z=+4.2)"},{"feature":"new_device","value":1,"reason":"Giao dịch từ thiết bị mới, chưa từng dùng với tài khoản này"}],"scenarios":[{"code":"ACCOUNT_TAKEOVER","severity":"high","title":"Nghi chiếm đoạt tài khoản","reason":"Thiết bị mới + người nhận mới + số tiền gấp 9.0 lần mức trung vị 30 ngày của tài khoản","evidence":["new_device","new_receiver","log_amount_to_type_median"]}]}`))
	response, err := decodeModelResponse(payload, testRiskSigningKey)
	if err != nil {
		t.Fatalf("trace fields from the ML service rejected: %v", err)
	}
	if len(response.TopFactors) != 2 || response.TopFactors[0].Reason == "" {
		t.Fatalf("top_factors not decoded: %+v", response.TopFactors)
	}
	if len(response.Scenarios) != 1 || response.Scenarios[0].Code != "ACCOUNT_TAKEOVER" || len(response.Scenarios[0].Evidence) != 3 {
		t.Fatalf("scenarios not decoded: %+v", response.Scenarios)
	}
}
