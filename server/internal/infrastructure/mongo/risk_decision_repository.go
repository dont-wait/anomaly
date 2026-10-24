package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/risk"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const riskDecisionsCollection = "risk_decisions"

const riskDecisionRetention = 24 * time.Hour

type RiskDecisionRepository struct {
	collection *mongodrv.Collection
	rejected   *mongodrv.Collection
}

func NewRiskDecisionRepository(client *mongodrv.Client, dbName string) *RiskDecisionRepository {
	return &RiskDecisionRepository{
		collection: client.Database(dbName).Collection(riskDecisionsCollection),
		rejected:   client.Database(dbName).Collection("risk_decision_rejections"),
	}
}

func (r *RiskDecisionRepository) EnsureIndexes(ctx context.Context) error {
	_, err := r.collection.Indexes().CreateOne(ctx, mongodrv.IndexModel{
		Keys:    bson.D{{Key: "transfer_id", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_risk_decision_transfer"),
	})
	if err != nil {
		return fmt.Errorf("create risk decision transfer index: %w", err)
	}
	_, err = r.collection.Indexes().CreateOne(ctx, mongodrv.IndexModel{
		Keys:    bson.D{{Key: "expires_at", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(0).SetName("ttl_risk_decision"),
	})
	if err != nil {
		return fmt.Errorf("create risk decision ttl index: %w", err)
	}
	_, err = r.rejected.Indexes().CreateOne(ctx, mongodrv.IndexModel{
		Keys:    bson.D{{Key: "expires_at", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(0).SetName("ttl_risk_decision_rejections"),
	})
	return err
}

func (r *RiskDecisionRepository) Save(ctx context.Context, decision risk.Decision) error {
	if decision.Assessment.Decision == "" {
		decision.Assessment.Decision = decision.Decision
	}
	if err := decision.Validate(); err != nil {
		return err
	}
	now := time.Now().UTC()
	if decision.CreatedAt.IsZero() {
		decision.CreatedAt = now
	}
	decision.UpdatedAt = now
	if decision.ExpiresAt.IsZero() {
		decision.ExpiresAt = now.Add(riskDecisionRetention)
	}
	// Insert once, never overwrite a finalized or terminal decision on redelivery.
	decision.Finalized, decision.FinalizedAt = false, nil
	_, err := r.collection.UpdateOne(ctx, bson.M{"transfer_id": decision.TransferID}, bson.M{
		"$setOnInsert": decision,
	}, options.UpdateOne().SetUpsert(true))
	if err != nil && !mongodrv.IsDuplicateKeyError(err) {
		return err
	}
	if decision.Decision == risk.DecisionPending {
		return nil
	}
	// A result may advance PENDING to terminal, but cannot regress it or replace
	// an already accepted terminal result. The predicate is checked atomically.
	_, err = r.collection.UpdateOne(ctx, bson.M{
		"transfer_id": decision.TransferID, "source_event_id": decision.SourceEventID,
		"finalized": false, "decision": risk.DecisionPending,
	}, bson.M{
		"$set": bson.M{
			"source_event_id": decision.SourceEventID,
			"decision":        decision.Decision,
			"assessment":      decision.Assessment,
			"updated_at":      decision.UpdatedAt,
			"expires_at":      decision.ExpiresAt,
		},
	})
	return err
}

func (r *RiskDecisionRepository) Find(ctx context.Context, transferID string) (risk.Decision, bool, error) {
	var decision risk.Decision
	err := r.collection.FindOne(ctx, bson.M{"transfer_id": transferID, "finalized": false, "expires_at": bson.M{"$gt": time.Now().UTC()}}).Decode(&decision)
	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return risk.Decision{}, false, nil
	}
	if err != nil {
		return risk.Decision{}, false, err
	}
	return decision, true, nil
}

func (r *RiskDecisionRepository) Finalize(ctx context.Context, transferID string) error {
	now := time.Now().UTC()
	_, err := r.collection.UpdateOne(ctx, bson.M{"transfer_id": transferID, "finalized": false}, bson.M{
		"$set": bson.M{"finalized": true, "finalized_at": now, "updated_at": now, "expires_at": now.Add(riskDecisionRetention)},
	})
	return err
}

// Reject parks invalid model responses durably before their Kafka offset is
// committed. Kafka coordinates make retries idempotent; raw payloads are never logged.
func (r *RiskDecisionRepository) Reject(ctx context.Context, topic string, partition int, offset int64, payload []byte, reason string) error {
	now := time.Now().UTC()
	_, err := r.rejected.UpdateOne(ctx, bson.M{"_id": fmt.Sprintf("%s:%d:%d", topic, partition, offset)}, bson.M{
		"$setOnInsert": bson.M{
			"topic": topic, "partition": partition, "offset": offset,
			"payload": payload, "reason": reason, "created_at": now,
			"expires_at": now.Add(7 * 24 * time.Hour),
		},
	}, options.UpdateOne().SetUpsert(true))
	return err
}
