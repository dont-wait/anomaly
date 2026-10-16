package mongo

import (
	"context"
	"errors"
	"time"

	maildomain "github.com/dont-wait/anomaly/internal/domain/mail"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	receiptOutboxPending = "pending"
	receiptOutboxSending = "sending"
	receiptOutboxFailed  = "failed"
	receiptOutboxSent    = "sent"
)

type ReceiptOutboxRepository struct {
	col *mongodrv.Collection
}

func NewReceiptOutboxRepository(client *mongodrv.Client, dbName string) *ReceiptOutboxRepository {
	return &ReceiptOutboxRepository{col: client.Database(dbName).Collection("receipt_outbox")}
}

func (r *ReceiptOutboxRepository) EnsureIndexes(ctx context.Context) error {
	collections, err := r.col.Database().ListCollectionNames(ctx, bson.M{"name": r.col.Name()})
	if err != nil {
		return err
	}
	if len(collections) == 0 {
		return errors.New("receipt_outbox collection is missing; run migrations before starting the API")
	}
	_, err = r.col.Indexes().CreateMany(ctx, []mongodrv.IndexModel{
		{
			Keys:    bson.D{{Key: "status", Value: 1}, {Key: "next_attempt_at", Value: 1}},
			Options: options.Index().SetName("idx_receipt_outbox_pending_retry"),
		},
	})
	return err
}

func (r *ReceiptOutboxRepository) Enqueue(ctx context.Context, transferID string, message maildomain.MailMessage) error {
	now := time.Now().UTC()
	_, err := r.col.UpdateOne(ctx, bson.M{"_id": transferID}, bson.M{
		"$setOnInsert": bson.M{
			"_id":             transferID,
			"to":              message.To,
			"subject":         message.Subject,
			"text":            message.Text,
			"html":            message.HTML,
			"status":          receiptOutboxPending,
			"attempts":        0,
			"next_attempt_at": now,
			"created_at":      now,
		},
	}, options.UpdateOne().SetUpsert(true))
	return err
}

func (r *ReceiptOutboxRepository) Claim(ctx context.Context, now time.Time) (maildomain.ReceiptOutboxItem, error) {
	staleBefore := now.Add(-5 * time.Minute)
	filter := bson.M{
		"$or": []bson.M{
			{"status": receiptOutboxPending, "next_attempt_at": bson.M{"$lte": now}},
			{"status": receiptOutboxFailed, "next_attempt_at": bson.M{"$lte": now}},
			{"status": receiptOutboxSending, "locked_at": bson.M{"$lte": staleBefore}},
		},
	}
	update := bson.M{
		"$set": bson.M{
			"status":    receiptOutboxSending,
			"locked_at": now,
			"lock_id":   uuid.NewString(),
		},
		"$inc": bson.M{"attempts": 1},
	}
	var item maildomain.ReceiptOutboxItem
	err := r.col.FindOneAndUpdate(ctx, filter, update, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&item)
	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return maildomain.ReceiptOutboxItem{}, nil
	}
	return item, err
}

func (r *ReceiptOutboxRepository) MarkSent(ctx context.Context, id, lockID string, sentAt time.Time) error {
	_, err := r.col.UpdateOne(ctx, bson.M{"_id": id, "status": receiptOutboxSending, "lock_id": lockID}, bson.M{
		"$set":   bson.M{"status": receiptOutboxSent, "sent_at": sentAt, "last_error": ""},
		"$unset": bson.M{"locked_at": ""},
	})
	return err
}

func (r *ReceiptOutboxRepository) MarkFailed(ctx context.Context, id, lockID string, err error, nextAttemptAt time.Time) error {
	lastError := "unknown receipt delivery error"
	if err != nil {
		lastError = err.Error()
	}
	_, updateErr := r.col.UpdateOne(ctx, bson.M{"_id": id, "status": receiptOutboxSending, "lock_id": lockID}, bson.M{
		"$set": bson.M{
			"status":          receiptOutboxFailed,
			"last_error":      lastError,
			"next_attempt_at": nextAttemptAt,
		},
		"$unset": bson.M{"locked_at": ""},
	})
	return updateErr
}
