package mongo

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const notificationDispatcherCheckpointID = "notification-dispatcher"

// NotificationCheckpoint stores the notification dispatcher's banking stream
// revision. It is independent from the banking projection checkpoint, so a
// projection replay never re-sends notifications.
type NotificationCheckpoint struct {
	col *mongodrv.Collection
}

func NewNotificationCheckpoint(client *mongodrv.Client, dbName string) *NotificationCheckpoint {
	return &NotificationCheckpoint{col: client.Database(dbName).Collection("checkpoints")}
}

func (c *NotificationCheckpoint) Position(ctx context.Context) (uint64, bool, error) {
	var row struct {
		Revision uint64 `bson:"revision"`
	}
	err := c.col.FindOne(ctx, bson.M{"_id": notificationDispatcherCheckpointID}).Decode(&row)
	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return 0, false, nil
	}
	return row.Revision, err == nil, err
}

// Save only moves forward; an older revision is ignored.
func (c *NotificationCheckpoint) Save(ctx context.Context, revision uint64) error {
	_, err := c.col.UpdateOne(ctx,
		bson.M{"_id": notificationDispatcherCheckpointID},
		bson.M{"$max": bson.M{"revision": revision}, "$set": bson.M{"updated_at": time.Now().UTC()}},
		options.UpdateOne().SetUpsert(true))
	return err
}
