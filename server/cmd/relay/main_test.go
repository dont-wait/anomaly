package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/modelfeature"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/google/uuid"
	esdb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
)

func TestSanitizePayloadRemovesSensitiveFields(t *testing.T) {
	got, err := sanitizePayload([]byte(`{"account":{"PasswordHash":"secret","username":"demo"},"nested":{"access_token":"token"},"transfer":{"Source":{"AccountNo":"ACC001","Name":"Alice"}}}`), "test-key")
	if err != nil {
		t.Fatalf("sanitizePayload() error = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("sanitized payload is invalid JSON: %v", err)
	}
	if _, ok := payload["account"]; ok {
		t.Fatal("account aggregate should be removed")
	}
	transfer := payload["transfer"].(map[string]any)
	source := transfer["Source"].(map[string]any)
	if source["AccountNo"] == "ACC001" || source["Name"] != nil {
		t.Fatalf("sensitive transfer fields were not protected: %#v", source)
	}
}

func TestSanitizePayloadRejectsInvalidJSON(t *testing.T) {
	if _, err := sanitizePayload([]byte(`not-json`), ""); err == nil {
		t.Fatal("sanitizePayload() error = nil, want error")
	}
}

type relayFeatureStoreFake struct {
	state       modelfeature.State
	decision    modelfeature.Decision
	decisionSet bool
	saved       bool
}

func (f *relayFeatureStoreFake) FindDecision(context.Context, string) (modelfeature.Decision, bool, error) {
	return f.decision, f.decisionSet, nil
}

func (f *relayFeatureStoreFake) SaveDecision(_ context.Context, decision modelfeature.Decision) error {
	f.decision = decision
	f.decisionSet = true
	return nil
}

func (f *relayFeatureStoreFake) LoadState(context.Context, string) (modelfeature.State, bool, error) {
	return f.state, f.state.AccountID != "", nil
}

func (f *relayFeatureStoreFake) IsKnownReceiver(context.Context, string, string, time.Time) (bool, error) {
	return false, nil
}

func (f *relayFeatureStoreFake) IsKnownDevice(context.Context, string, string, time.Time) (bool, error) {
	return false, nil
}

func (f *relayFeatureStoreFake) ApplyState(_ context.Context, _ modelfeature.Transaction, state modelfeature.State) error {
	f.state = state
	f.saved = true
	return nil
}

func (f *relayFeatureStoreFake) LoadProgress(context.Context) (uint64, bool, error) {
	return 0, false, nil
}

func (f *relayFeatureStoreFake) SaveProgress(context.Context, string, uint64) error {
	return nil
}

func (f *relayFeatureStoreFake) ResetForBootstrap(context.Context) error {
	return nil
}

func (f *relayFeatureStoreFake) CleanupAppliedDecisions(context.Context, time.Time) error {
	return nil
}

func TestBuildEnrichedEnvelopeAddsModelFeatures(t *testing.T) {
	eventID := uuid.New()
	transactionID := uuid.New()
	at := time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)
	sourceID := uuid.NewString()
	destinationID := uuid.NewString()
	amount := int64(50000000)
	fee := int64(0)
	sourceBefore, sourceAfter := int64(100000000), int64(50000000)
	destinationBefore, destinationAfter := int64(10000000), int64(60000000)
	event := journal.Event{
		ID:            eventID.String(),
		TransactionID: transactionID.String(),
		Type:          journal.Completed,
		SchemaVersion: journal.SchemaVersion,
		Sequence:      2,
		OccurredAt:    at,
		Payload: journal.Payload{
			SourceAccountID:          sourceID,
			DestinationAccountID:     destinationID,
			Amount:                   amount,
			Fee:                      fee,
			Currency:                 "VND",
			Channel:                  "mobile",
			Status:                   "success",
			SourceBalanceBefore:      &sourceBefore,
			SourceBalanceAfter:       &sourceAfter,
			DestinationBalanceBefore: &destinationBefore,
			DestinationBalanceAfter:  &destinationAfter,
		},
	}
	record := journal.Record{
		ID: event.ID, Type: event.Type, At: at, Event: &event,
		Transfer: json.RawMessage(`{"ID":"` + transactionID.String() + `","DeviceID":"DEVICE01"}`),
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}
	store := &relayFeatureStoreFake{}
	envelope, pending, err := buildEnrichedEnvelope(context.Background(), &esdb.RecordedEvent{
		EventID: eventID, EventType: journal.Completed, StreamID: journal.BankStream,
		EventNumber: 1, CreatedDate: at, Data: data,
	}, store, "test-redaction-key")
	if err != nil {
		t.Fatalf("buildEnrichedEnvelope() error = %v", err)
	}
	if pending == nil || envelope.ModelFeatures == nil {
		t.Fatalf("envelope = %#v, pending = %#v; want model features", envelope, pending)
	}
	if envelope.ModelFeatures.AvgAmount7d != 0 || envelope.ModelFeatures.TxCount1h != 0 || !envelope.ModelFeatures.NewReceiver || !envelope.ModelFeatures.NewDevice {
		t.Fatalf("model features = %#v", envelope.ModelFeatures)
	}
	if !store.decisionSet {
		t.Fatal("feature decision was not saved before publish")
	}
}
