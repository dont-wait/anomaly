package mongo

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type RecipientContact struct {
	ID                 bson.ObjectID `bson:"_id,omitempty"`
	OwnerAccountID     bson.ObjectID `bson:"owner_account_id"`
	RecipientAccountNo string        `bson:"recipient_account_no"`
	RecipientName      string        `bson:"recipient_name"`
	BankCode           string        `bson:"bank_code"`
	CreatedAt          time.Time     `bson:"created_at"`
	UpdatedAt          time.Time     `bson:"updated_at"`
}

type RecipientContactRepository struct {
	col *mongodrv.Collection
}

func NewRecipientContactRepository(client *mongodrv.Client, dbName string) *RecipientContactRepository {
	return &RecipientContactRepository{col: client.Database(dbName).Collection("recipient_contacts")}
}

func (r *RecipientContactRepository) EnsureIndexes(ctx context.Context) error {
	collections, err := r.col.Database().ListCollectionNames(ctx, bson.M{"name": r.col.Name()})
	if err != nil {
		return err
	}
	if len(collections) == 0 {
		return fmt.Errorf("recipient_contacts collection is missing; run migrations before starting the API")
	}
	_, err = r.col.Indexes().CreateMany(ctx, []mongodrv.IndexModel{
		{
			Keys: bson.D{
				{Key: "owner_account_id", Value: 1},
				{Key: "bank_code", Value: 1},
				{Key: "recipient_account_no", Value: 1},
			},
			Options: options.Index().SetUnique(true).SetName("uniq_recipient_contact_owner_target"),
		},
		{
			Keys:    bson.D{{Key: "owner_account_id", Value: 1}, {Key: "recipient_name", Value: 1}},
			Options: options.Index().SetName("idx_recipient_contact_owner_name"),
		},
	})
	return err
}

func (r *RecipientContactRepository) List(ctx context.Context, ownerID bson.ObjectID, query string) ([]RecipientContact, error) {
	filter := bson.M{"owner_account_id": ownerID}
	if query = strings.TrimSpace(query); query != "" {
		pattern := bson.M{"$regex": regexp.QuoteMeta(query), "$options": "i"}
		filter["$or"] = []bson.M{
			{"recipient_name": pattern},
			{"recipient_account_no": pattern},
			{"bank_code": pattern},
		}
	}

	cursor, err := r.col.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "recipient_name", Value: 1}, {Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	contacts := make([]RecipientContact, 0)
	if err := cursor.All(ctx, &contacts); err != nil {
		return nil, err
	}
	return contacts, nil
}

func (r *RecipientContactRepository) Add(ctx context.Context, ownerID bson.ObjectID, accountNo, name, bankCode string) (bool, error) {
	now := time.Now().UTC()
	filter := bson.M{
		"owner_account_id":     ownerID,
		"recipient_account_no": accountNo,
		"bank_code":            bankCode,
	}
	update := bson.M{
		"$set": bson.M{
			"recipient_name": name,
			"updated_at":     now,
		},
		"$setOnInsert": bson.M{
			"owner_account_id":     ownerID,
			"recipient_account_no": accountNo,
			"bank_code":            bankCode,
			"created_at":           now,
		},
	}
	result, err := r.col.UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(true))
	if IsDuplicateKeyError(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return result.UpsertedCount > 0, nil
}

func (r *RecipientContactRepository) Delete(ctx context.Context, ownerID bson.ObjectID, accountNo, bankCode string) (bool, error) {
	result, err := r.col.DeleteOne(ctx, bson.M{
		"owner_account_id":     ownerID,
		"recipient_account_no": accountNo,
		"bank_code":            bankCode,
	})
	if err != nil {
		return false, err
	}
	return result.DeletedCount > 0, nil
}
