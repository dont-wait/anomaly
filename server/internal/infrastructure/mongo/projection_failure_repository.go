package mongo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const ProjectionFailureStatusPending = "pending"

type ProjectionFailure struct {
	EventID     string
	StreamID    string
	EventType   string
	EventNumber uint64
	Commit      uint64
	Prepare     uint64
	Reason      string
	Error       string
	FailedAt    time.Time
}

type ProjectionFailureRepository struct {
	col *mongodrv.Collection
}

func NewProjectionFailureRepository(client *mongodrv.Client, dbName string) *ProjectionFailureRepository {
	return &ProjectionFailureRepository{col: client.Database(dbName).Collection("projection_failures")}
}

func (r *ProjectionFailureRepository) EnsureIndexes(ctx context.Context) error {
	_, err := r.col.Indexes().CreateOne(ctx, mongodrv.IndexModel{
		Keys:    bson.D{{Key: "status", Value: 1}, {Key: "failed_at", Value: -1}},
		Options: options.Index().SetName("idx_status_failed_at"),
	})
	return err
}

// Save upsert theo event ID để việc event được giao lại không tạo nhiều
// dead-letter giống nhau. Payload không được sao chép sang Mongo vì event có
// thể chứa dữ liệu nhạy cảm; EventStore vẫn là nguồn để replay theo event ID.
func (r *ProjectionFailureRepository) Save(ctx context.Context, failure ProjectionFailure) error {
	_, err := r.col.UpdateOne(
		ctx,
		bson.M{"_id": failure.EventID},
		bson.M{
			"$setOnInsert": bson.M{
				"stream_id":    failure.StreamID,
				"event_type":   failure.EventType,
				"event_number": failure.EventNumber,
				"commit":       failure.Commit,
				"prepare":      failure.Prepare,
				"status":       ProjectionFailureStatusPending,
				"failed_at":    failure.FailedAt,
			},
			"$set": bson.M{
				"reason":         failure.Reason,
				"error":          failure.Error,
				"last_failed_at": failure.FailedAt,
			},
			"$inc": bson.M{"attempts": 1},
		},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}
