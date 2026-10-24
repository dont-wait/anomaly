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

type RiskDecisionRepository struct {
	collection *mongodrv.Collection
}

func NewRiskDecisionRepository(client *mongodrv.Client, dbName string) *RiskDecisionRepository {
	return &RiskDecisionRepository{collection: client.Database(dbName).Collection(riskDecisionsCollection)}
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
	return nil
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
		decision.ExpiresAt = now.Add(24 * time.Hour)
	}
	_, err := r.collection.UpdateOne(ctx, bson.M{"transfer_id": decision.TransferID}, bson.M{
		"$set": bson.M{
			"source_event_id": decision.SourceEventID,
			"decision":        decision.Decision,
			"assessment":      decision.Assessment,
			"updated_at":      decision.UpdatedAt,
			"expires_at":      decision.ExpiresAt,
		},
		"$setOnInsert": bson.M{
			"transfer_id": decision.TransferID,
			"created_at":  decision.CreatedAt,
			"finalized":   false,
		},
	}, options.UpdateOne().SetUpsert(true))
	return err
}

func (r *RiskDecisionRepository) Find(ctx context.Context, transferID string) (risk.Decision, bool, error) {
	var decision risk.Decision
	err := r.collection.FindOne(ctx, bson.M{"transfer_id": transferID, "finalized": false}).Decode(&decision)
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
	_, err := r.collection.UpdateOne(ctx, bson.M{"transfer_id": transferID}, bson.M{
		"$set": bson.M{"finalized": true, "finalized_at": now, "updated_at": now},
	})
	return err
}
