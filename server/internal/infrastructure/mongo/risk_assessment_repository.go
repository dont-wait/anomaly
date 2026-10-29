package mongo

import (
	"context"
	"errors"
	"fmt"

	"github.com/dont-wait/anomaly/internal/domain/riskassessment"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const riskAssessmentsCollection = "risk_assessments"

var ErrRiskAssessmentConflict = errors.New("risk assessment idempotency conflict")

type RiskAssessmentRepository struct {
	collection *mongodrv.Collection
}

func NewRiskAssessmentRepository(client *mongodrv.Client, dbName string) *RiskAssessmentRepository {
	return &RiskAssessmentRepository{
		collection: client.Database(dbName).Collection(riskAssessmentsCollection),
	}
}

func (r *RiskAssessmentRepository) EnsureIndexes(ctx context.Context) error {
	_, err := r.collection.Indexes().CreateOne(ctx, mongodrv.IndexModel{
		Keys:    bson.D{{Key: "transaction_id", Value: 1}, {Key: "model_version", Value: 1}, {Key: "feature_version", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_risk_assessment_model_input"),
	})
	if err != nil {
		return fmt.Errorf("create risk assessment idempotency index: %w", err)
	}
	_, err = r.collection.Indexes().CreateOne(ctx, mongodrv.IndexModel{
		Keys:    bson.D{{Key: "received_at", Value: 1}, {Key: "_id", Value: 1}},
		Options: options.Index().SetName("risk_assessment_received_at"),
	})
	return err
}

// Save inserts an assessment once. Kafka redelivery is a successful no-op.
func (r *RiskAssessmentRepository) Save(ctx context.Context, assessment riskassessment.Assessment) (bool, error) {
	if assessment.ID == "" {
		return false, errors.New("risk assessment id is required")
	}
	_, err := r.collection.InsertOne(ctx, assessment)
	if err == nil {
		return true, nil
	}
	if mongodrv.IsDuplicateKeyError(err) {
		var existing riskassessment.Assessment
		lookupErr := r.collection.FindOne(ctx, bson.M{
			"transaction_id":  assessment.TransactionID,
			"model_version":   assessment.ModelVersion,
			"feature_version": assessment.FeatureVersion,
		}).Decode(&existing)
		if lookupErr == nil && existing.RiskScore == assessment.RiskScore && existing.PredictedLabel == assessment.PredictedLabel && existing.Threshold == assessment.Threshold {
			return false, nil
		}
		if lookupErr != nil && !errors.Is(lookupErr, mongodrv.ErrNoDocuments) {
			return false, lookupErr
		}
		return false, ErrRiskAssessmentConflict
	}
	return false, err
}
