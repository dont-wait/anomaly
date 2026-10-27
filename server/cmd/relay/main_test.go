package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/modelfeature"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/google/uuid"
	esdb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
)

func TestBuildEnvelopePreservesOriginalPayload(t *testing.T) {
	data := []byte(`{ "account":{"PasswordHash":"secret","username":"demo"},"nested":{"access_token":"token"},"transfer":{"Source":{"AccountNo":"001234567890","Name":"Nguyễn Văn A"},"DeviceID":"DEVICE01","Note":"test <transfer>","Amount":9007199254740993,"optional":null} }`)
	recorded := &esdb.RecordedEvent{EventID: uuid.New(), EventType: journal.Created, StreamID: journal.BankStream, Data: data}
	envelope, err := buildEnvelope(recorded)
	if err != nil {
		t.Fatalf("buildEnvelope() error = %v", err)
	}
	if !bytes.Equal(envelope.Payload, data) {
		t.Fatalf("payload changed: %s", envelope.Payload)
	}
	// Verify serialization to the Kafka envelope preserves every field/value,
	// including large integers and strings, without float64 decoding or redaction.
	wire, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	var expected any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&expected); err != nil {
		t.Fatal(err)
	}
	expectedJSON, err := json.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	var actual any
	decoder = json.NewDecoder(bytes.NewReader(decoded.Payload))
	decoder.UseNumber()
	if err := decoder.Decode(&actual); err != nil {
		t.Fatal(err)
	}
	actualJSON, err := json.Marshal(actual)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(expectedJSON, actualJSON) {
		t.Fatalf("serialized payload values changed: %s", decoded.Payload)
	}
}

func TestBuildEnvelopeRejectsInvalidJSON(t *testing.T) {
	for _, data := range []string{"", "not-json", `{} {}`} {
		if _, err := buildEnvelope(&esdb.RecordedEvent{Data: []byte(data)}); err == nil {
			t.Fatalf("buildEnvelope(%q) error = nil, want error", data)
		}
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

func (f *relayFeatureStoreFake) FindDecisionByTransactionID(context.Context, string) (modelfeature.Decision, bool, error) {
	return f.decision, f.decisionSet, nil
}

func (f *relayFeatureStoreFake) SaveDecision(_ context.Context, decision modelfeature.Decision) error {
	f.decision = decision
	f.decisionSet = true
	return nil
}

func (f *relayFeatureStoreFake) DiscardDecisionsByTransactionID(context.Context, string) error {
	f.decisionSet = false
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

func (f *relayFeatureStoreFake) CleanupAppliedDecisions(context.Context, uint64, time.Time) error {
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
	}, store)
	if err != nil {
		t.Fatalf("buildEnrichedEnvelope() error = %v", err)
	}
	if pending == nil || envelope.ModelFeatures == nil {
		t.Fatalf("envelope = %#v, pending = %#v; want model features", envelope, pending)
	}
	if !bytes.Equal(envelope.Payload, data) {
		t.Fatal("feature enrichment changed the original payload")
	}
	if envelope.ModelFeatures.AvgAmount7d != 0 || envelope.ModelFeatures.TxCount1h != 0 || !envelope.ModelFeatures.NewReceiver || !envelope.ModelFeatures.NewDevice {
		t.Fatalf("model features = %#v", envelope.ModelFeatures)
	}
	if !store.decisionSet {
		t.Fatal("feature decision was not saved before publish")
	}
	if store.decision.StreamRevision != 1 {
		t.Fatalf("decision stream revision = %d, want 1", store.decision.StreamRevision)
	}
}

func TestCreatedEnvelopePreservesAccountAndDeviceFields(t *testing.T) {
	id := uuid.NewString()
	at := time.Now().UTC()
	event := journal.Event{
		ID: journal.EventID(id, journal.Created), TransactionID: id, Type: journal.Created,
		SchemaVersion: journal.SchemaVersion, Sequence: 1, OccurredAt: at,
		Payload: journal.Payload{
			SourceAccountID: "source-id", DestinationAccountID: "destination-id",
			Amount: 1000, Currency: "VND", Channel: "mobile", Status: "awaiting_otp",
		},
	}
	record := journal.Record{
		ID: event.ID, Type: event.Type, At: at, Event: &event,
		Transfer: json.RawMessage(`{"ID":"` + id + `","Source":{"AccountNo":"001234567890","Name":"Sender"},"Destination":{"AccountNo":"009876543210","Name":"Recipient"},"DeviceID":"DEVICE01","Note":"original note"}`),
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	store := &relayFeatureStoreFake{}
	envelope, pending, err := buildEnrichedEnvelope(context.Background(), &esdb.RecordedEvent{
		EventID: uuid.MustParse(event.ID), EventType: journal.Created, StreamID: journal.BankStream, Data: data,
	}, store)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(envelope.Payload, data) {
		t.Fatal("created payload was modified")
	}
	if envelope.ModelFeatures == nil || pending != nil || store.saved {
		t.Fatal("created must enrich features without applying rolling state")
	}
}
