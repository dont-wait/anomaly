package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/risk"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
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
