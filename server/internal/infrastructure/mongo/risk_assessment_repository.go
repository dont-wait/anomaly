package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

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

type RiskAssessmentBucket struct {
	Timestamp       time.Time `bson:"_id"`
	AssessmentCount int       `bson:"assessment_count"`
	AnomalousCount  int       `bson:"anomalous_count"`
	ScoreTotal      float64   `bson:"score_total"`
}

type RiskAssessmentVersionCount struct {
	Label string `bson:"_id"`
	Count int    `bson:"count"`
}

type RiskAssessmentSummary struct {
	Total         int64
	Anomalous     int64
	ScoreTotal    float64
	Timeseries    []RiskAssessmentBucket
	ModelVersions []RiskAssessmentVersionCount
}

type riskAssessmentChange struct {
	OperationType string                    `bson:"operationType"`
	FullDocument  riskassessment.Assessment `bson:"fullDocument"`
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

func (r *RiskAssessmentRepository) List(ctx context.Context, from, to time.Time, limit int64) ([]riskassessment.Assessment, error) {
	if limit <= 0 {
		limit = 1000
	}
	if limit > 10000 {
		limit = 10000
	}
	cursor, err := r.collection.Find(ctx, bson.M{
		"received_at": bson.M{"$gte": from, "$lt": to},
	}, options.Find().SetSort(bson.D{{Key: "received_at", Value: 1}, {Key: "_id", Value: 1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	var items []riskassessment.Assessment
	for cursor.Next(ctx) {
		var item riskassessment.Assessment
		if err := cursor.Decode(&item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *RiskAssessmentRepository) ListRecent(ctx context.Context, from, to time.Time, limit int64) ([]riskassessment.Assessment, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	cursor, err := r.collection.Find(ctx, bson.M{
		"received_at": bson.M{"$gte": from, "$lt": to},
	}, options.Find().SetSort(bson.D{{Key: "received_at", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	var items []riskassessment.Assessment
	for cursor.Next(ctx) {
		var item riskassessment.Assessment
		if err := cursor.Decode(&item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
	return items, nil
}

func (r *RiskAssessmentRepository) Summarize(ctx context.Context, from, to time.Time) (RiskAssessmentSummary, error) {
	filter := bson.M{"received_at": bson.M{"$gte": from, "$lt": to}}
	total, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return RiskAssessmentSummary{}, err
	}
	anomalous, err := r.collection.CountDocuments(ctx, bson.M{
		"received_at":     bson.M{"$gte": from, "$lt": to},
		"predicted_label": 1,
	})
	if err != nil {
		return RiskAssessmentSummary{}, err
	}

	summary := RiskAssessmentSummary{Total: total, Anomalous: anomalous}
	scoreCursor, err := r.collection.Aggregate(ctx, mongodrv.Pipeline{
		bson.D{{Key: "$match", Value: filter}},
		bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "score_total", Value: bson.D{{Key: "$sum", Value: "$risk_score"}}}}}},
	})
	if err != nil {
		return RiskAssessmentSummary{}, err
	}
	var scoreRow struct {
		ScoreTotal float64 `bson:"score_total"`
	}
	if scoreCursor.Next(ctx) {
		if err := scoreCursor.Decode(&scoreRow); err != nil {
			_ = scoreCursor.Close(ctx)
			return RiskAssessmentSummary{}, err
		}
	}
	if err := scoreCursor.Err(); err != nil {
		_ = scoreCursor.Close(ctx)
		return RiskAssessmentSummary{}, err
	}
	_ = scoreCursor.Close(ctx)
	summary.ScoreTotal = scoreRow.ScoreTotal

	seriesCursor, err := r.collection.Aggregate(ctx, mongodrv.Pipeline{
		bson.D{{Key: "$match", Value: filter}},
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{{Key: "$dateTrunc", Value: bson.D{{Key: "date", Value: "$received_at"}, {Key: "unit", Value: "hour"}}}}},
			{Key: "assessment_count", Value: bson.D{{Key: "$sum", Value: 1}}},
			{Key: "anomalous_count", Value: bson.D{{Key: "$sum", Value: bson.D{{Key: "$cond", Value: bson.A{bson.D{{Key: "$eq", Value: bson.A{"$predicted_label", 1}}}, 1, 0}}}}}},
			{Key: "score_total", Value: bson.D{{Key: "$sum", Value: "$risk_score"}}},
		}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	})
	if err != nil {
		return RiskAssessmentSummary{}, err
	}
	defer func() { _ = seriesCursor.Close(ctx) }()
	for seriesCursor.Next(ctx) {
		var row RiskAssessmentBucket
		if err := seriesCursor.Decode(&row); err != nil {
			return RiskAssessmentSummary{}, err
		}
		summary.Timeseries = append(summary.Timeseries, row)
	}
	if err := seriesCursor.Err(); err != nil {
		return RiskAssessmentSummary{}, err
	}
	if summary.Timeseries == nil {
		summary.Timeseries = []RiskAssessmentBucket{}
	}

	versionCursor, err := r.collection.Aggregate(ctx, mongodrv.Pipeline{
		bson.D{{Key: "$match", Value: filter}},
		bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$model_version"}, {Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	})
	if err != nil {
		return RiskAssessmentSummary{}, err
	}
	defer func() { _ = versionCursor.Close(ctx) }()
	for versionCursor.Next(ctx) {
		var row RiskAssessmentVersionCount
		if err := versionCursor.Decode(&row); err != nil {
			return RiskAssessmentSummary{}, err
		}
		summary.ModelVersions = append(summary.ModelVersions, row)
	}
	if err := versionCursor.Err(); err != nil {
		return RiskAssessmentSummary{}, err
	}
	if summary.ModelVersions == nil {
		summary.ModelVersions = []RiskAssessmentVersionCount{}
	}
	return summary, nil
}

func (r *RiskAssessmentRepository) ListAfter(ctx context.Context, after time.Time, afterID string, limit int64) ([]riskassessment.Assessment, error) {
	if limit <= 0 {
		limit = 1000
	}
	if limit > 10000 {
		limit = 10000
	}
	filter := bson.M{}
	if !after.IsZero() || afterID != "" {
		filter = bson.M{"$or": bson.A{
			bson.M{"received_at": bson.M{"$gt": after}},
			bson.M{"received_at": after, "_id": bson.M{"$gt": afterID}},
		}}
	}
	cursor, err := r.collection.Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "received_at", Value: 1}, {Key: "_id", Value: 1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	var items []riskassessment.Assessment
	for cursor.Next(ctx) {
		var item riskassessment.Assessment
		if err := cursor.Decode(&item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// Watch emits persisted assessments for other API instances as well as the
// local instance. The durable collection remains the source of truth; a
// reconnecting client can replay from its cursor through ListAfter.
func (r *RiskAssessmentRepository) Watch(ctx context.Context, resumeAfter bson.Raw, publish func(riskassessment.Assessment)) (bson.Raw, error) {
	changeStreamOptions := options.ChangeStream()
	if len(resumeAfter) > 0 {
		changeStreamOptions.SetResumeAfter(resumeAfter)
	}
	stream, err := r.collection.Watch(ctx, mongodrv.Pipeline{}, changeStreamOptions)
	if err != nil {
		return resumeAfter, err
	}
	defer func() { _ = stream.Close(ctx) }()
	for stream.Next(ctx) {
		var change riskAssessmentChange
		if err := stream.Decode(&change); err != nil {
			return stream.ResumeToken(), err
		}
		if change.OperationType == "insert" && change.FullDocument.ID != "" {
			publish(change.FullDocument)
		}
	}
	return stream.ResumeToken(), stream.Err()
}
