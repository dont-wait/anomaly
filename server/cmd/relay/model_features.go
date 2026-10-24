package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/modelfeature"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	kafkainfra "github.com/dont-wait/anomaly/internal/infrastructure/kafka"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	esdb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
)

type modelFeatureStore interface {
	FindDecision(context.Context, string) (modelfeature.Decision, bool, error)
	FindDecisionByTransactionID(context.Context, string) (modelfeature.Decision, bool, error)
	SaveDecision(context.Context, modelfeature.Decision) error
	LoadState(context.Context, string) (modelfeature.State, bool, error)
	IsKnownReceiver(context.Context, string, string, time.Time) (bool, error)
	IsKnownDevice(context.Context, string, string, time.Time) (bool, error)
	ApplyState(context.Context, modelfeature.Transaction, modelfeature.State) error
	LoadProgress(context.Context) (uint64, bool, error)
	SaveProgress(context.Context, string, uint64) error
	ResetForBootstrap(context.Context) error
	CleanupAppliedDecisions(context.Context, uint64, time.Time) error
}

type pendingFeatureUpdate struct {
	Transaction modelfeature.Transaction
	NextState   modelfeature.State
}

type retryableFeatureError struct {
	err error
}

func (e *retryableFeatureError) Error() string {
	return e.err.Error()
}

func (e *retryableFeatureError) Unwrap() error {
	return e.err
}

func retryableFeatureStoreError(err error) error {
	if err == nil {
		return nil
	}
	return &retryableFeatureError{err: err}
}

type transferFeatureSnapshot struct {
	DeviceID string `json:"DeviceID"`
}

func buildEnrichedEnvelope(ctx context.Context, recorded *esdb.RecordedEvent, store modelFeatureStore) (kafkainfra.EventEnvelope, *pendingFeatureUpdate, error) {
	envelope, err := buildEnvelope(recorded)
	if err != nil {
		return kafkainfra.EventEnvelope{}, nil, err
	}
	if recorded.EventType != journal.Created && recorded.EventType != journal.Completed {
		return envelope, nil, nil
	}
	if store == nil {
		return kafkainfra.EventEnvelope{}, nil, fmt.Errorf("model feature store is required for completed transfer")
	}

	record, err := journal.DecodeRecord(recorded.Data, recorded.EventID.String(), recorded.EventType)
	if err != nil {
		return kafkainfra.EventEnvelope{}, nil, err
	}
	if record.Event == nil {
		return kafkainfra.EventEnvelope{}, nil, fmt.Errorf("transfer event is missing")
	}

	var snapshot transferFeatureSnapshot
	if err := json.Unmarshal(record.Transfer, &snapshot); err != nil {
		return kafkainfra.EventEnvelope{}, nil, fmt.Errorf("decode transfer feature snapshot: %w", err)
	}
	transaction := modelfeature.Transaction{
		EventID:         record.Event.ID,
		TransactionID:   record.Event.TransactionID,
		StreamRevision:  recorded.EventNumber,
		SourceAccountID: record.Event.Payload.SourceAccountID,
		DestinationID:   record.Event.Payload.DestinationAccountID,
		DeviceID:        strings.TrimSpace(snapshot.DeviceID),
		Amount:          record.Event.Payload.Amount,
		OccurredAt:      record.Event.OccurredAt,
	}
	transactionID := record.Event.TransactionID

	state, _, err := store.LoadState(ctx, transaction.SourceAccountID)
	if err != nil {
		return kafkainfra.EventEnvelope{}, nil, retryableFeatureStoreError(fmt.Errorf("load model feature state: %w", err))
	}
	decision, found, err := store.FindDecision(ctx, transaction.EventID)
	if err != nil {
		return kafkainfra.EventEnvelope{}, nil, retryableFeatureStoreError(fmt.Errorf("load model feature decision: %w", err))
	}
	if recorded.EventType == journal.Completed && !found {
		decision, found, err = store.FindDecisionByTransactionID(ctx, transactionID)
		if err != nil {
			return kafkainfra.EventEnvelope{}, nil, retryableFeatureStoreError(fmt.Errorf("load created model feature decision: %w", err))
		}
	}

	var features modelfeature.Features
	var nextState modelfeature.State
	if found {
		features = decision.Features
		_, nextState, err = modelfeature.Calculate(state, transaction, true, true)
	} else {
		receiverKnown, lookupErr := store.IsKnownReceiver(ctx, transaction.SourceAccountID, transaction.DestinationID, transaction.OccurredAt)
		if lookupErr != nil {
			return kafkainfra.EventEnvelope{}, nil, retryableFeatureStoreError(fmt.Errorf("check known receiver: %w", lookupErr))
		}
		deviceKnown := false
		if transaction.DeviceID != "" && !strings.EqualFold(transaction.DeviceID, "unknown") {
			deviceKnown, lookupErr = store.IsKnownDevice(ctx, transaction.SourceAccountID, transaction.DeviceID, transaction.OccurredAt)
			if lookupErr != nil {
				return kafkainfra.EventEnvelope{}, nil, retryableFeatureStoreError(fmt.Errorf("check known device: %w", lookupErr))
			}
		}
		features, nextState, err = modelfeature.Calculate(state, transaction, receiverKnown, deviceKnown)
		if err == nil {
			err = store.SaveDecision(ctx, modelfeature.Decision{
				EventID:        transaction.EventID,
				TransactionID:  transactionID,
				StreamRevision: transaction.StreamRevision,
				Features:       features,
			})
			if err != nil {
				err = retryableFeatureStoreError(fmt.Errorf("save model feature decision: %w", err))
			}
		}
	}
	if err != nil {
		return kafkainfra.EventEnvelope{}, nil, fmt.Errorf("calculate model features: %w", err)
	}

	envelope.ModelFeatures = &features
	if recorded.EventType == journal.Created {
		return envelope, nil, nil
	}
	// Store a decision under the completed event id as well so the existing
	// atomic ApplyState operation can commit the state exactly once.
	if err := store.SaveDecision(ctx, modelfeature.Decision{
		EventID:        transaction.EventID,
		TransactionID:  transactionID,
		StreamRevision: transaction.StreamRevision,
		Features:       features,
	}); err != nil {
		return kafkainfra.EventEnvelope{}, nil, retryableFeatureStoreError(fmt.Errorf("save completed model feature decision: %w", err))
	}
	return envelope, &pendingFeatureUpdate{Transaction: transaction, NextState: nextState}, nil
}

var _ modelFeatureStore = (*mongorepo.ModelFeatureRepository)(nil)
