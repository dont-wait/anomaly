package mongo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/modelfeature"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	modelFeatureStatesCollection    = "model_feature_states"
	modelFeatureDecisionsCollection = "model_feature_decisions"
	modelKnownReceiversCollection   = "model_known_receivers"
	modelKnownDevicesCollection     = "model_known_devices"
	modelFeatureProgressCollection  = "model_feature_progress"
	modelFeatureProgressID          = "relay"
	modelDecisionIndexName          = "idx_model_decision_state"
)

var ErrFeatureStateConflict = errors.New("model feature state changed concurrently")

type ModelFeatureRepository struct {
	client    *mongodrv.Client
	states    *mongodrv.Collection
	decisions *mongodrv.Collection
	receivers *mongodrv.Collection
	devices   *mongodrv.Collection
	progress  *mongodrv.Collection
}

func NewModelFeatureRepository(client *mongodrv.Client, dbName string) *ModelFeatureRepository {
	db := client.Database(dbName)
	return &ModelFeatureRepository{
		client:    client,
		states:    db.Collection(modelFeatureStatesCollection),
		decisions: db.Collection(modelFeatureDecisionsCollection),
		receivers: db.Collection(modelKnownReceiversCollection),
		devices:   db.Collection(modelKnownDevicesCollection),
		progress:  db.Collection(modelFeatureProgressCollection),
	}
}

func (r *ModelFeatureRepository) EnsureIndexes(ctx context.Context) error {
	_, err := r.states.Indexes().CreateOne(ctx, mongodrv.IndexModel{
		Keys:    bson.D{{Key: "state_version", Value: 1}},
		Options: options.Index().SetName("idx_model_feature_state_version"),
	})
	if err != nil {
		return fmt.Errorf("create model feature state index: %w", err)
	}
	_, err = r.receivers.Indexes().CreateOne(ctx, mongodrv.IndexModel{
		Keys:    bson.D{{Key: "account_id", Value: 1}, {Key: "receiver_id", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_model_receiver"),
	})
	if err != nil {
		return fmt.Errorf("create model receiver index: %w", err)
	}
	_, err = r.devices.Indexes().CreateOne(ctx, mongodrv.IndexModel{
		Keys:    bson.D{{Key: "account_id", Value: 1}, {Key: "device_id", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_model_device"),
	})
	if err != nil {
		return fmt.Errorf("create model device index: %w", err)
	}
	// Older relay versions used the same name with only state_applied. Drop
	// that definition first so startup performs the index migration itself.
	if err := r.decisions.Indexes().DropOne(ctx, modelDecisionIndexName); err != nil {
		var commandErr mongodrv.CommandError
		if !errors.As(err, &commandErr) || commandErr.Code != 27 {
			return fmt.Errorf("replace model decision index: %w", err)
		}
	}
	if _, err = r.decisions.Indexes().CreateOne(ctx, mongodrv.IndexModel{
		Keys:    bson.D{{Key: "state_applied", Value: 1}, {Key: "applied_at", Value: 1}},
		Options: options.Index().SetName(modelDecisionIndexName),
	}); err != nil {
		return fmt.Errorf("create model decision index: %w", err)
	}
	return nil
}

func (r *ModelFeatureRepository) FindDecision(ctx context.Context, eventID string) (modelfeature.Decision, bool, error) {
	var decision modelfeature.Decision
	err := r.decisions.FindOne(ctx, bson.M{"_id": eventID}).Decode(&decision)
	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return modelfeature.Decision{}, false, nil
	}
	if err != nil {
		return modelfeature.Decision{}, false, err
	}
	return decision, true, nil
}

func (r *ModelFeatureRepository) SaveDecision(ctx context.Context, decision modelfeature.Decision) error {
	if decision.EventID == "" {
		return fmt.Errorf("model feature decision event id is required")
	}
	_, err := r.decisions.UpdateOne(ctx, bson.M{"_id": decision.EventID}, bson.M{
		"$setOnInsert": bson.M{
			"_id":             decision.EventID,
			"stream_revision": decision.StreamRevision,
			"features":        decision.Features,
			"state_applied":   false,
		},
	}, options.UpdateOne().SetUpsert(true))
	return err
}

func (r *ModelFeatureRepository) LoadState(ctx context.Context, accountID string) (modelfeature.State, bool, error) {
	var state modelfeature.State
	err := r.states.FindOne(ctx, bson.M{"_id": accountID}).Decode(&state)
	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return modelfeature.State{AccountID: accountID}, false, nil
	}
	if err != nil {
		return modelfeature.State{}, false, err
	}
	return state, true, nil
}

func (r *ModelFeatureRepository) IsKnownReceiver(ctx context.Context, accountID, receiverID string, at time.Time) (bool, error) {
	return r.existsAt(ctx, r.receivers, bson.M{"account_id": accountID, "receiver_id": receiverID}, at)
}

func (r *ModelFeatureRepository) IsKnownDevice(ctx context.Context, accountID, deviceID string, at time.Time) (bool, error) {
	return r.existsAt(ctx, r.devices, bson.M{"account_id": accountID, "device_id": deviceID}, at)
}

func (r *ModelFeatureRepository) existsAt(ctx context.Context, collection *mongodrv.Collection, filter bson.M, at time.Time) (bool, error) {
	filter["$or"] = bson.A{
		bson.M{"first_seen_at": bson.M{"$lte": at}},
		bson.M{"first_seen_at": bson.M{"$exists": false}},
	}
	err := collection.FindOne(ctx, filter).Err()
	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

type featureProgress struct {
	ID             string    `bson:"_id"`
	StreamRevision uint64    `bson:"stream_revision"`
	LastEventID    string    `bson:"last_event_id"`
	UpdatedAt      time.Time `bson:"updated_at"`
}

func (r *ModelFeatureRepository) LoadProgress(ctx context.Context) (uint64, bool, error) {
	var progress featureProgress
	err := r.progress.FindOne(ctx, bson.M{"_id": modelFeatureProgressID}).Decode(&progress)
	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return progress.StreamRevision, true, nil
}

func (r *ModelFeatureRepository) SaveProgress(ctx context.Context, eventID string, revision uint64) error {
	_, err := r.progress.UpdateOne(ctx, bson.M{"_id": modelFeatureProgressID}, bson.M{
		"$max":         bson.M{"stream_revision": revision},
		"$set":         bson.M{"last_event_id": eventID, "updated_at": time.Now().UTC()},
		"$setOnInsert": bson.M{"_id": modelFeatureProgressID},
	}, options.UpdateOne().SetUpsert(true))
	return err
}

func (r *ModelFeatureRepository) ResetForBootstrap(ctx context.Context) error {
	for _, collection := range []*mongodrv.Collection{r.states, r.decisions, r.receivers, r.devices, r.progress} {
		if _, err := collection.DeleteMany(ctx, bson.M{}); err != nil {
			return fmt.Errorf("reset model feature collection: %w", err)
		}
	}
	return nil
}

func (r *ModelFeatureRepository) CleanupAppliedDecisions(ctx context.Context, checkpoint uint64, before time.Time) error {
	if checkpoint == 0 {
		return nil
	}
	_, err := r.decisions.DeleteMany(ctx, bson.M{
		"state_applied":   true,
		"stream_revision": bson.M{"$exists": true, "$lte": checkpoint},
		"applied_at":      bson.M{"$lt": before},
	})
	return err
}

// ApplyState atomically commits the rolling state, known receiver/device and
// decision marker after Kafka has accepted the enriched event.
func (r *ModelFeatureRepository) ApplyState(ctx context.Context, tx modelfeature.Transaction, state modelfeature.State) error {
	session, err := r.client.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, func(sc context.Context) (any, error) {
		var decision modelfeature.Decision
		if err := r.decisions.FindOne(sc, bson.M{"_id": tx.EventID}).Decode(&decision); err != nil {
			return nil, fmt.Errorf("load model feature decision: %w", err)
		}
		if decision.StateApplied {
			return nil, nil
		}

		if state.StateVersion <= 0 {
			return nil, fmt.Errorf("invalid model feature state version: %d", state.StateVersion)
		}
		expectedVersion := state.StateVersion - 1
		result, err := r.states.ReplaceOne(sc, bson.M{
			"_id":           state.AccountID,
			"state_version": expectedVersion,
		}, state, options.Replace().SetUpsert(true))
		if err != nil {
			return nil, fmt.Errorf("save model feature state: %w", err)
		}
		if result.MatchedCount == 0 && result.UpsertedCount == 0 {
			return nil, fmt.Errorf("%w: account=%s expected_version=%d", ErrFeatureStateConflict, state.AccountID, expectedVersion)
		}
		if tx.DestinationID != "" {
			if _, err := r.receivers.UpdateOne(sc, bson.M{"account_id": tx.SourceAccountID, "receiver_id": tx.DestinationID}, bson.M{
				"$setOnInsert": bson.M{"account_id": tx.SourceAccountID, "receiver_id": tx.DestinationID},
				"$min":         bson.M{"first_seen_at": tx.OccurredAt},
			}, options.UpdateOne().SetUpsert(true)); err != nil {
				return nil, fmt.Errorf("save known receiver: %w", err)
			}
		}
		if tx.DeviceID != "" && !strings.EqualFold(tx.DeviceID, "unknown") {
			if _, err := r.devices.UpdateOne(sc, bson.M{"account_id": tx.SourceAccountID, "device_id": tx.DeviceID}, bson.M{
				"$setOnInsert": bson.M{"account_id": tx.SourceAccountID, "device_id": tx.DeviceID},
				"$min":         bson.M{"first_seen_at": tx.OccurredAt},
			}, options.UpdateOne().SetUpsert(true)); err != nil {
				return nil, fmt.Errorf("save known device: %w", err)
			}
		}
		_, err = r.decisions.UpdateOne(sc, bson.M{"_id": tx.EventID, "state_applied": false}, bson.M{"$set": bson.M{"state_applied": true, "applied_at": time.Now().UTC()}})
		return nil, err
	})
	return err
}
