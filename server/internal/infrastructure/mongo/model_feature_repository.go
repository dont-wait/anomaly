package mongo

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
)

type ModelFeatureRepository struct {
	client    *mongodrv.Client
	states    *mongodrv.Collection
	decisions *mongodrv.Collection
	receivers *mongodrv.Collection
	devices   *mongodrv.Collection
}

func NewModelFeatureRepository(client *mongodrv.Client, dbName string) *ModelFeatureRepository {
	db := client.Database(dbName)
	return &ModelFeatureRepository{
		client:    client,
		states:    db.Collection(modelFeatureStatesCollection),
		decisions: db.Collection(modelFeatureDecisionsCollection),
		receivers: db.Collection(modelKnownReceiversCollection),
		devices:   db.Collection(modelKnownDevicesCollection),
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
	_, err = r.decisions.Indexes().CreateOne(ctx, mongodrv.IndexModel{
		Keys:    bson.D{{Key: "state_applied", Value: 1}},
		Options: options.Index().SetName("idx_model_decision_state"),
	})
	if err != nil {
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
			"_id":           decision.EventID,
			"features":      decision.Features,
			"state_applied": false,
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

func (r *ModelFeatureRepository) IsKnownReceiver(ctx context.Context, accountID, receiverID string) (bool, error) {
	return r.exists(ctx, r.receivers, bson.M{"account_id": accountID, "receiver_id": receiverID})
}

func (r *ModelFeatureRepository) IsKnownDevice(ctx context.Context, accountID, deviceID string) (bool, error) {
	return r.exists(ctx, r.devices, bson.M{"account_id": accountID, "device_id": deviceID})
}

func (r *ModelFeatureRepository) exists(ctx context.Context, collection *mongodrv.Collection, filter bson.M) (bool, error) {
	err := collection.FindOne(ctx, filter).Err()
	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
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

		if _, err := r.states.ReplaceOne(sc, bson.M{"_id": state.AccountID}, state, options.Replace().SetUpsert(true)); err != nil {
			return nil, fmt.Errorf("save model feature state: %w", err)
		}
		if tx.DestinationID != "" {
			if _, err := r.receivers.UpdateOne(sc, bson.M{"account_id": tx.SourceAccountID, "receiver_id": tx.DestinationID}, bson.M{
				"$setOnInsert": bson.M{"account_id": tx.SourceAccountID, "receiver_id": tx.DestinationID},
			}, options.UpdateOne().SetUpsert(true)); err != nil {
				return nil, fmt.Errorf("save known receiver: %w", err)
			}
		}
		if tx.DeviceID != "" && !strings.EqualFold(tx.DeviceID, "unknown") {
			if _, err := r.devices.UpdateOne(sc, bson.M{"account_id": tx.SourceAccountID, "device_id": tx.DeviceID}, bson.M{
				"$setOnInsert": bson.M{"account_id": tx.SourceAccountID, "device_id": tx.DeviceID},
			}, options.UpdateOne().SetUpsert(true)); err != nil {
				return nil, fmt.Errorf("save known device: %w", err)
			}
		}
		_, err := r.decisions.UpdateOne(sc, bson.M{"_id": tx.EventID, "state_applied": false}, bson.M{"$set": bson.M{"state_applied": true}})
		return nil, err
	})
	return err
}
