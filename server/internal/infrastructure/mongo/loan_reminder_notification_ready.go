package mongo

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// CheckReady checks schema created by migration 20 without creating or
// modifying indexes. Workers must not perform implicit schema migrations.
func (r *NotificationRepository) CheckReady(ctx context.Context) error {
	collections, err := r.col.Database().ListCollectionNames(ctx, bson.M{"name": r.col.Name()})
	if err != nil {
		return fmt.Errorf("inspect notifications collection: %w", err)
	}
	if len(collections) == 0 {
		return errors.New("notifications collection is missing (migration 20 not applied)")
	}
	cursor, err := r.col.Indexes().List(ctx)
	if err != nil {
		return fmt.Errorf("list notification indexes: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()
	found := make(map[string]bool)
	for cursor.Next(ctx) {
		var spec struct {
			Name string `bson:"name"`
		}
		if err := cursor.Decode(&spec); err != nil {
			return fmt.Errorf("decode notification index specification: %w", err)
		}
		found[spec.Name] = true
	}
	if err := cursor.Err(); err != nil {
		return fmt.Errorf("iterate notification indexes: %w", err)
	}
	for _, name := range []string{"idx_notifications_inbox", "idx_notifications_unread", "idx_notifications_ttl"} {
		if !found[name] {
			return fmt.Errorf("required notification index %s is missing (migration 20 incomplete)", name)
		}
	}
	return nil
}
