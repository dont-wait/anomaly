package mongo

import (
	"context"
	"errors"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/notification"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const notificationsCollection = "notifications"

type NotificationRepository struct {
	col *mongodrv.Collection
}

func NewNotificationRepository(client *mongodrv.Client, dbName string) *NotificationRepository {
	return &NotificationRepository{col: client.Database(dbName).Collection(notificationsCollection)}
}

type notificationDocument struct {
	ID            string            `bson:"_id"`
	Audience      string            `bson:"audience"`
	RecipientID   string            `bson:"recipient_id"`
	Type          string            `bson:"type"`
	Category      string            `bson:"category"`
	Severity      string            `bson:"severity"`
	Title         string            `bson:"title"`
	Body          string            `bson:"body"`
	Data          map[string]string `bson:"data"`
	SourceEventID string            `bson:"source_event_id"`
	OccurredAt    time.Time         `bson:"occurred_at"`
	CreatedAt     time.Time         `bson:"created_at"`
	ReadAt        *time.Time        `bson:"read_at"`
	ExpiresAt     time.Time         `bson:"expires_at"`
}

func toNotificationDocument(n notification.Notification) notificationDocument {
	data := n.Data
	if data == nil {
		data = map[string]string{}
	}
	return notificationDocument{
		ID: n.ID, Audience: string(n.Audience), RecipientID: n.RecipientID, Type: string(n.Type),
		Category: string(n.Category), Severity: string(n.Severity), Title: n.Title, Body: n.Body,
		Data: data, SourceEventID: n.SourceEventID, OccurredAt: n.OccurredAt.UTC(),
		CreatedAt: n.CreatedAt.UTC(), ReadAt: n.ReadAt, ExpiresAt: n.ExpiresAt.UTC(),
	}
}

func (d notificationDocument) toDomain() notification.Notification {
	return notification.Notification{
		ID: d.ID, Audience: notification.Audience(d.Audience), RecipientID: d.RecipientID,
		Type: notification.Type(d.Type), Category: notification.Category(d.Category),
		Severity: notification.Severity(d.Severity), Title: d.Title, Body: d.Body, Data: d.Data,
		SourceEventID: d.SourceEventID, OccurredAt: d.OccurredAt.UTC(), CreatedAt: d.CreatedAt.UTC(),
		ReadAt: d.ReadAt, ExpiresAt: d.ExpiresAt.UTC(),
	}
}

func (r *NotificationRepository) EnsureIndexes(ctx context.Context) error {
	collections, err := r.col.Database().ListCollectionNames(ctx, bson.M{"name": r.col.Name()})
	if err != nil {
		return err
	}
	if len(collections) == 0 {
		return errors.New("notifications collection is missing; run migrations before starting the API")
	}
	_, err = r.col.Indexes().CreateMany(ctx, []mongodrv.IndexModel{
		{
			Keys: bson.D{
				{Key: "audience", Value: 1},
				{Key: "recipient_id", Value: 1},
				{Key: "created_at", Value: -1},
				{Key: "_id", Value: -1},
			},
			Options: options.Index().SetName("idx_notifications_inbox"),
		},
		{
			Keys: bson.D{{Key: "audience", Value: 1}, {Key: "recipient_id", Value: 1}, {Key: "read_at", Value: 1}},
			Options: options.Index().SetName("idx_notifications_unread").
				SetPartialFilterExpression(bson.M{"read_at": bson.M{"$type": "null"}}),
		},
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetName("idx_notifications_ttl").SetExpireAfterSeconds(0),
		},
	})
	return err
}

// InsertMany upserts with $setOnInsert so an existing notification (same
// deterministic id) is left untouched, including its read state.
func (r *NotificationRepository) InsertMany(ctx context.Context, items []notification.Notification) error {
	if len(items) == 0 {
		return nil
	}
	models := make([]mongodrv.WriteModel, 0, len(items))
	for _, item := range items {
		models = append(models, mongodrv.NewUpdateOneModel().
			SetFilter(bson.M{"_id": item.ID}).
			SetUpdate(bson.M{"$setOnInsert": toNotificationDocument(item)}).
			SetUpsert(true))
	}
	_, err := r.col.BulkWrite(ctx, models, options.BulkWrite().SetOrdered(false))
	return err
}

func recipientFilter(audience notification.Audience, recipientID string) bson.M {
	return bson.M{"audience": string(audience), "recipient_id": recipientID}
}

// List returns newest first, strictly older than before when it is set.
func (r *NotificationRepository) List(ctx context.Context, audience notification.Audience, recipientID string, before *notification.Cursor, limit int, unreadOnly bool) ([]notification.Notification, error) {
	filter := recipientFilter(audience, recipientID)
	if unreadOnly {
		filter["read_at"] = bson.M{"$type": "null"}
	}
	if before != nil {
		filter["$or"] = bson.A{
			bson.M{"created_at": bson.M{"$lt": before.CreatedAt}},
			bson.M{"created_at": before.CreatedAt, "_id": bson.M{"$lt": before.ID}},
		}
	}
	sort := bson.D{{Key: "created_at", Value: -1}, {Key: "_id", Value: -1}}
	return r.find(ctx, filter, sort, limit)
}

// ListAfter returns oldest first, strictly newer than after, for SSE replay.
func (r *NotificationRepository) ListAfter(ctx context.Context, audience notification.Audience, recipientID string, after notification.Cursor, limit int) ([]notification.Notification, error) {
	filter := recipientFilter(audience, recipientID)
	filter["$or"] = bson.A{
		bson.M{"created_at": bson.M{"$gt": after.CreatedAt}},
		bson.M{"created_at": after.CreatedAt, "_id": bson.M{"$gt": after.ID}},
	}
	sort := bson.D{{Key: "created_at", Value: 1}, {Key: "_id", Value: 1}}
	return r.find(ctx, filter, sort, limit)
}

func (r *NotificationRepository) find(ctx context.Context, filter bson.M, sort bson.D, limit int) ([]notification.Notification, error) {
	cursor, err := r.col.Find(ctx, filter, options.Find().SetSort(sort).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	items := make([]notification.Notification, 0, limit)
	for cursor.Next(ctx) {
		var doc notificationDocument
		if err := cursor.Decode(&doc); err != nil {
			return nil, err
		}
		items = append(items, doc.toDomain())
	}
	return items, cursor.Err()
}

func (r *NotificationRepository) CountUnread(ctx context.Context, audience notification.Audience, recipientID string) (int64, error) {
	filter := recipientFilter(audience, recipientID)
	filter["read_at"] = bson.M{"$type": "null"}
	return r.col.CountDocuments(ctx, filter)
}

// MarkRead is idempotent for the owner and reports notification.ErrNotFound
// both for a missing id and for an id owned by someone else.
func (r *NotificationRepository) MarkRead(ctx context.Context, audience notification.Audience, recipientID, id string, at time.Time) error {
	filter := recipientFilter(audience, recipientID)
	filter["_id"] = id
	result, err := r.col.UpdateOne(ctx, filter, bson.A{
		bson.M{"$set": bson.M{"read_at": bson.M{"$ifNull": bson.A{"$read_at", at.UTC()}}}},
	})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return notification.ErrNotFound
	}
	return nil
}

func (r *NotificationRepository) MarkAllRead(ctx context.Context, audience notification.Audience, recipientID string, before, at time.Time) (int64, error) {
	filter := recipientFilter(audience, recipientID)
	filter["read_at"] = bson.M{"$type": "null"}
	filter["created_at"] = bson.M{"$lte": before.UTC()}
	result, err := r.col.UpdateMany(ctx, filter, bson.M{"$set": bson.M{"read_at": at.UTC()}})
	if err != nil {
		return 0, err
	}
	return result.ModifiedCount, nil
}

// Watch emits inserted notifications so every API instance can push to the
// SSE clients it holds. It returns the last resume token on exit.
func (r *NotificationRepository) Watch(ctx context.Context, resumeAfter bson.Raw, startAtOperationTime *bson.Timestamp, publish func(notification.Notification)) (bson.Raw, error) {
	changeStreamOptions := options.ChangeStream()
	if len(resumeAfter) > 0 {
		changeStreamOptions.SetResumeAfter(resumeAfter)
	} else if startAtOperationTime != nil {
		changeStreamOptions.SetStartAtOperationTime(startAtOperationTime)
	}
	pipeline := mongodrv.Pipeline{{{Key: "$match", Value: bson.M{"operationType": "insert"}}}}
	stream, err := r.col.Watch(ctx, pipeline, changeStreamOptions)
	if err != nil {
		return resumeAfter, err
	}
	defer func() { _ = stream.Close(ctx) }()
	for stream.Next(ctx) {
		var change struct {
			FullDocument notificationDocument `bson:"fullDocument"`
		}
		if err := stream.Decode(&change); err != nil {
			return stream.ResumeToken(), err
		}
		if change.FullDocument.ID != "" {
			publish(change.FullDocument.toDomain())
		}
	}
	return stream.ResumeToken(), stream.Err()
}
