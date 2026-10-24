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
}

func (f *riskResponseStoreFake) Save(context.Context, risk.Decision) error {
	f.saves++
	return f.saveErr
}

func (f *riskResponseStoreFake) Reject(_ context.Context, topic string, partition int, offset int64, payload []byte, _ string) error {
	f.rejects++
	f.parked = kafkago.Message{Topic: topic, Partition: partition, Offset: offset, Value: payload}
	return f.rejectErr
}

func validRiskResponse(t *testing.T) []byte {
	t.Helper()
	id := uuid.NewString()
	payload, err := json.Marshal(map[string]any{
		"transfer_id": id, "source_event_id": journal.EventID(id, journal.Created), "decision": risk.DecisionAllow,
		"risk_score": 0.08, "is_anomalous": false, "model_version": "v1", "evaluated_at": time.Now().UTC(),
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
		if err := handleRiskDecision(context.Background(), message, store, zerolog.Nop()); err != nil {
			t.Fatalf("parked poison message must permit offset commit: %v", err)
		}
		if store.rejects != 1 || store.saves != 0 || store.parked.Offset != 41 || string(store.parked.Value) != string(payload) {
			t.Fatalf("invalid quarantine: %+v", store)
		}
		message.Offset++
		message.Value = validRiskResponse(t)
		if err := handleRiskDecision(context.Background(), message, store, zerolog.Nop()); err != nil || store.saves != 1 {
			t.Fatalf("valid message after poison was not saved: %v", err)
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
			err := handleRiskDecision(context.Background(), kafkago.Message{Value: test.payload}, test.store, zerolog.Nop())
			if !errors.Is(err, failure) {
				t.Fatalf("storage failure must prevent commit, got %v", err)
			}
		})
	}
}
