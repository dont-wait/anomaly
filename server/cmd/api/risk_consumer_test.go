package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/risk"
	"github.com/dont-wait/anomaly/internal/domain/riskassessment"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	kafkago "github.com/segmentio/kafka-go"
)

type riskResponseStoreFake struct {
	saves, rejects     int
	saveErr, rejectErr error
	parked             kafkago.Message
	saved              risk.Decision
}

func (f *riskResponseStoreFake) Save(_ context.Context, decision risk.Decision) error {
	f.saves++
	f.saved = decision
	return f.saveErr
}

func (f *riskResponseStoreFake) Reject(_ context.Context, topic string, partition int, offset int64, payload []byte, _ string) error {
	f.rejects++
	f.parked = kafkago.Message{Topic: topic, Partition: partition, Offset: offset, Value: payload}
	return f.rejectErr
}

const testRiskSigningKey = "test-risk-signing-key"

func TestDecodeRiskDecisionAcceptsPythonPublisherVectors(t *testing.T) {
	data, err := os.ReadFile("../../../anomaly-service/tests/fixtures/risk_decisions.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Key       string          `json:"key"`
		Message   json.RawMessage `json:"message"`
		Canonical string          `json:"canonical"`
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	for _, vector := range vectors {
		var unsigned riskDecisionUnsigned
		if err := json.Unmarshal(vector.Message, &unsigned); err != nil {
			t.Fatal(err)
		}
		canonical, err := json.Marshal(unsigned)
		if err != nil || string(canonical) != vector.Canonical {
			t.Fatalf("Python/Go decision canonical JSON differs: Go=%s Python=%s err=%v", canonical, vector.Canonical, err)
		}
		decisions := &riskResponseStoreFake{}
		assessments := &riskAssessmentStoreFake{}
		if err := handleRiskResponse(context.Background(), kafkago.Message{Value: vector.Message}, decisions, assessments, zerolog.Nop(), vector.Key); err != nil {
			t.Fatal(err)
		}
		if decisions.saves != 1 || decisions.rejects != 0 || assessments.saves != 0 || decisions.saved.Decision != unsigned.Decision {
			t.Fatal("Python decision was not routed to the existing transfer decision flow")
		}
		if _, err := decodeRiskDecision(vector.Message, "wrong-key"); err == nil {
			t.Fatal("Python decision accepted with wrong key")
		}
	}
}

func signedModelResponse(t *testing.T, payload []byte) []byte {
	t.Helper()
	var response riskassessment.ModelResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatal(err)
	}
	fields["signature"] = signModelResponse(response, testRiskSigningKey)
	result, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func validModelResponse(t *testing.T) []byte {
	return signedModelResponse(t, []byte(`{"transaction_id":"TX001","risk_score":0.98,"predicted_label":1,"threshold":0.95,"model_version":"model-v1","feature_version":"features-v1"}`))
}

type riskAssessmentStoreFake struct {
	saves int
	saved riskassessment.Assessment
	err   error
}

func (f *riskAssessmentStoreFake) Save(_ context.Context, assessment riskassessment.Assessment) (bool, error) {
	f.saves++
	f.saved = assessment
	return true, f.err
}

func TestRiskResponseAssessmentNeverWritesTransferDecision(t *testing.T) {
	decisions := &riskResponseStoreFake{}
	assessments := &riskAssessmentStoreFake{}
	message := kafkago.Message{Topic: "risk", Partition: 2, Offset: 41, Value: validModelResponse(t)}
	if err := handleRiskResponse(context.Background(), message, decisions, assessments, zerolog.Nop(), testRiskSigningKey); err != nil {
		t.Fatal(err)
	}
	if decisions.saves != 0 || decisions.rejects != 0 || assessments.saves != 1 {
		t.Fatalf("assessment incorrectly routed: decisions=%+v assessments=%+v", decisions, assessments)
	}
	if assessments.saved.TransactionID != "TX001" || assessments.saved.PredictedLabel != 1 || assessments.saved.KafkaOffset != 41 {
		t.Fatalf("incorrect saved assessment: %+v", assessments.saved)
	}
}

func TestRiskResponseKeepsExistingSignedDecisionFlow(t *testing.T) {
	decisions := &riskResponseStoreFake{}
	assessments := &riskAssessmentStoreFake{}
	if err := handleRiskResponse(context.Background(), kafkago.Message{Value: validRiskResponse(t)}, decisions, assessments, zerolog.Nop(), testRiskSigningKey); err != nil {
		t.Fatal(err)
	}
	if decisions.saves != 1 || decisions.rejects != 0 || assessments.saves != 0 {
		t.Fatal("existing decision flow changed")
	}
}

func TestRiskResponseParksUnsignedAndTamperedAssessments(t *testing.T) {
	for _, signature := range []string{"", "forged"} {
		var fields map[string]any
		if err := json.Unmarshal(validModelResponse(t), &fields); err != nil {
			t.Fatal(err)
		}
		if signature == "" {
			delete(fields, "signature")
		} else {
			fields["signature"] = signature
		}
		payload, _ := json.Marshal(fields)
		decisions := &riskResponseStoreFake{}
		assessments := &riskAssessmentStoreFake{}
		if err := handleRiskResponse(context.Background(), kafkago.Message{Value: payload}, decisions, assessments, zerolog.Nop(), testRiskSigningKey); err != nil {
			t.Fatal(err)
		}
		if decisions.rejects != 1 || decisions.saves != 0 || assessments.saves != 0 {
			t.Fatal("untrusted assessment was not quarantined")
		}
	}
	var fields map[string]any
	if err := json.Unmarshal(validModelResponse(t), &fields); err != nil {
		t.Fatal(err)
	}
	fields["risk_score"] = 0.99 // Still a valid label; only HMAC should reject it.
	payload, _ := json.Marshal(fields)
	if _, err := decodeModelResponse(payload, testRiskSigningKey); err == nil {
		t.Fatal("tampered score accepted")
	}
	if _, err := decodeModelResponse(validModelResponse(t), ""); err == nil {
		t.Fatal("empty signing key accepted")
	}
}

func TestAssessmentStorageFailuresRemainRetryable(t *testing.T) {
	failure := errors.New("storage unavailable")
	for _, test := range []struct {
		name        string
		payload     []byte
		decisions   *riskResponseStoreFake
		assessments *riskAssessmentStoreFake
	}{
		{"save", validModelResponse(t), &riskResponseStoreFake{}, &riskAssessmentStoreFake{err: failure}},
		{"reject", []byte(`{"transaction_id":"TX001"}`), &riskResponseStoreFake{rejectErr: failure}, &riskAssessmentStoreFake{}},
		{"conflict reject", validModelResponse(t), &riskResponseStoreFake{rejectErr: failure}, &riskAssessmentStoreFake{err: mongorepo.ErrRiskAssessmentConflict}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := handleRiskResponse(context.Background(), kafkago.Message{Value: test.payload}, test.decisions, test.assessments, zerolog.Nop(), testRiskSigningKey)
			if !errors.Is(err, failure) {
				t.Fatalf("storage failure must prevent offset commit: %v", err)
			}
		})
	}
	decisions := &riskResponseStoreFake{}
	if err := handleRiskResponse(context.Background(), kafkago.Message{Value: validModelResponse(t)}, decisions, &riskAssessmentStoreFake{err: mongorepo.ErrRiskAssessmentConflict}, zerolog.Nop(), testRiskSigningKey); err != nil || decisions.rejects != 1 {
		t.Fatalf("conflict must be parked: %v", err)
	}
}

// These vectors are emitted by anomaly-service's Python publisher. Verify with
// the actual server decoder so field order, floats, escaping and timestamps agree.
func TestDecodeModelResponseAcceptsPythonPublisherVectors(t *testing.T) {
	data, err := os.ReadFile("../../../anomaly-service/tests/fixtures/model_responses.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Key       string          `json:"key"`
		Message   json.RawMessage `json:"message"`
		Canonical string          `json:"canonical"`
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	for _, vector := range vectors {
		var unsigned riskassessment.ModelResponse
		if err := json.Unmarshal(vector.Message, &unsigned); err != nil {
			t.Fatal(err)
		}
		canonical, err := json.Marshal(unsigned)
		if err != nil || string(canonical) != vector.Canonical {
			t.Fatalf("Python/Go canonical JSON differs: Go=%s Python=%s err=%v", canonical, vector.Canonical, err)
		}
		decision, err := decodeModelResponse(vector.Message, vector.Key)
		if err != nil {
			t.Fatalf("Python publisher decision rejected: %v", err)
		}
		if decision != unsigned {
			t.Fatalf("unexpected decision: %+v", decision)
		}
		if _, err := decodeModelResponse(vector.Message, "wrong-key"); err == nil {
			t.Fatal("Python publisher signature accepted with wrong key")
		}
	}
}

func validRiskResponse(t *testing.T) []byte {
	t.Helper()
	id := uuid.NewString()
	unsigned := riskDecisionUnsigned{
		TransferID: id, SourceEventID: journal.EventID(id, journal.Created), Decision: risk.DecisionAllow,
		IsAnomalous: false, RiskScore: 0.08, ModelVersion: "v1", EvaluatedAt: time.Now().UTC(),
	}
	payload, err := json.Marshal(riskDecisionEnvelope{
		TransferID: unsigned.TransferID, SourceEventID: unsigned.SourceEventID, Decision: unsigned.Decision,
		IsAnomalous: &unsigned.IsAnomalous, RiskScore: &unsigned.RiskScore,
		ModelVersion: unsigned.ModelVersion, EvaluatedAt: unsigned.EvaluatedAt,
		Signature: signRiskDecision(unsigned, testRiskSigningKey),
	})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestRiskConsumerParksPoisonAndContinuesWithValidMessage(t *testing.T) {
	for _, payload := range [][]byte{[]byte(`{`), []byte(`{"decision":"UNKNOWN"}`), []byte(`{"transfer_id":"bad","source_event_id":"bad","decision":"PENDING"}`)} {
		store := &riskResponseStoreFake{}
		message := kafkago.Message{Topic: "risk", Partition: 2, Offset: 41, Value: payload}
		if err := handleRiskDecision(context.Background(), message, store, zerolog.Nop(), testRiskSigningKey); err != nil {
			t.Fatalf("parked poison message must permit offset commit: %v", err)
		}
		if store.rejects != 1 || store.saves != 0 || store.parked.Offset != 41 || string(store.parked.Value) != string(payload) {
			t.Fatalf("invalid quarantine: %+v", store)
		}
		message.Offset++
		message.Value = validRiskResponse(t)
		if err := handleRiskDecision(context.Background(), message, store, zerolog.Nop(), testRiskSigningKey); err != nil || store.saves != 1 {
			t.Fatalf("valid message after poison was not saved: %v", err)
		}
		if store.saved.TransferID == "" || store.saved.Decision != risk.DecisionAllow || store.saved.Assessment.RiskScore != 0.08 {
			t.Fatalf("saved decision was not decoded: %+v", store.saved)
		}
	}
}

func TestRiskConsumerDoesNotCommitOnStorageFailure(t *testing.T) {
	failure := errors.New("storage unavailable")
	for _, test := range []struct {
		name    string
		payload []byte
		store   *riskResponseStoreFake
	}{
		{"parking failure", []byte(`{`), &riskResponseStoreFake{rejectErr: failure}},
		{"save failure", validRiskResponse(t), &riskResponseStoreFake{saveErr: failure}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := handleRiskDecision(context.Background(), kafkago.Message{Value: test.payload}, test.store, zerolog.Nop(), testRiskSigningKey)
			if !errors.Is(err, failure) {
				t.Fatalf("storage failure must prevent commit, got %v", err)
			}
		})
	}
}

func TestDecodeRiskDecisionRejectsMissingAssessmentFields(t *testing.T) {
	id := uuid.NewString()
	payload, err := json.Marshal(map[string]any{
		"transfer_id": id, "source_event_id": journal.EventID(id, journal.Created),
		"decision": risk.DecisionAllow, "model_version": "v1", "evaluated_at": time.Now().UTC(),
		"signature": "not-valid",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeRiskDecision(payload, testRiskSigningKey); err == nil {
		t.Fatal("missing assessment fields were accepted")
	}
}

func TestDecodeRiskDecisionRejectsForgedSignature(t *testing.T) {
	payload := validRiskResponse(t)
	var envelope riskDecisionEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope.Decision = risk.DecisionBlock
	forged, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeRiskDecision(forged, testRiskSigningKey); err == nil {
		t.Fatal("forged risk decision was accepted")
	}
}

func TestDecodeModelResponse(t *testing.T) {
	payload := []byte(`{"transaction_id":"TX001","risk_score":0.9821,"predicted_label":1,"threshold":0.9517361521720886,"model_version":"gatv2-device-control-v1","feature_version":"schema-3-0b9efd44d793"}`)
	payload = signedModelResponse(t, payload)
	response, err := decodeModelResponse(payload, testRiskSigningKey)
	if err != nil {
		t.Fatalf("valid model response rejected: %v", err)
	}
	if response.TransactionID != "TX001" || response.PredictedLabel != 1 || response.ModelVersion == "" {
		t.Fatalf("unexpected model response: %+v", response)
	}
}

func TestDecodeModelResponseRejectsUnknownAndTrailingFields(t *testing.T) {
	base := `{"transaction_id":"TX001","risk_score":0.1,"predicted_label":0,"threshold":0.9,"model_version":"model-v1","feature_version":"features-v1"}`
	withExtra := signedModelResponse(t, []byte(`{"transaction_id":"TX001","risk_score":0.1,"predicted_label":0,"threshold":0.9,"model_version":"model-v1","feature_version":"features-v1","extra":true}`))
	for _, payload := range [][]byte{
		withExtra,
		append(signedModelResponse(t, []byte(base)), []byte(` {}`)...),
	} {
		if _, err := decodeModelResponse(payload, testRiskSigningKey); err == nil {
			t.Fatalf("invalid model response accepted: %s", payload)
		}
	}
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
