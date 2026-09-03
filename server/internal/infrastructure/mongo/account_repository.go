package mongo

import (
	"context"
	"errors"

	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"go.mongodb.org/mongo-driver/v2/bson"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type AccountRepository struct {
	col *mongodrv.Collection
}

func NewAccountRepository(client *mongodrv.Client, dbName string) *AccountRepository {
	return NewAccountRepositoryFromCollection(client.Database(dbName).Collection("accounts"))
}

func NewAccountRepositoryFromCollection(col *mongodrv.Collection) *AccountRepository {
	return &AccountRepository{col: col}
}

// EnsureIndexes tạo unique index cho `email` và `username` — bảo vệ account
// repository khỏi race duplicate khi hai request register đồng thời vượt
// qua check ở application layer (Mongo read model eventually consistent).
// Idempotent — an toàn gọi nhiều lần.
//
// Trả về lỗi nếu Mongo không khả dụng, caller quyết định retry hay fail.
func (r *AccountRepository) EnsureIndexes(ctx context.Context) error {
	indexes := []mongodrv.IndexModel{
		{
			Keys:    bson.D{{Key: "account_no", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uniq_account_no"),
		},
		{
			Keys:    bson.D{{Key: "email", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uniq_email"),
		},
		{
			Keys:    bson.D{{Key: "username", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uniq_username"),
		},
		{
			Keys:    bson.D{{Key: "customer_id", Value: 1}},
			Options: options.Index().SetName("idx_customer_id"),
		},
	}
	_, err := r.col.Indexes().CreateMany(ctx, indexes)
	return err
}

// IsDuplicateKeyError trả về true nếu err là duplicate key error từ unique
// index. Worker dùng để nhận diện xung đột khi Mongo từ chối ghi projection.
func IsDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	const duplicateKeyCode = 11000
	var we mongodrv.WriteException
	if errors.As(err, &we) {
		return we.HasErrorCode(duplicateKeyCode)
	}
	var ce *mongodrv.CommandError
	if errors.As(err, &ce) {
		return ce.HasErrorCode(duplicateKeyCode)
	}
	return false
}

func (r *AccountRepository) Save(ctx context.Context, a *accountdomain.UserAccount) error {
	record, err := toRecord(a)
	if err != nil {
		return err
	}
	_, err = r.col.ReplaceOne(
		ctx,
		bson.M{"_id": record.Id},
		record,
		options.Replace().SetUpsert(true),
	)
	return err
}

func (r *AccountRepository) FindByID(ctx context.Context, id string) (*accountdomain.UserAccount, error) {
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, nil
	}
	var rec accountRecord
	err = r.col.FindOne(ctx, bson.M{"_id": objectID}).Decode(&rec)
	if err != nil {
		if err == mongodrv.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}

	return fromRecord(rec)
}

func (r *AccountRepository) FindByEmail(ctx context.Context, email string) (*accountdomain.UserAccount, error) {
	var rec accountRecord
	err := r.col.FindOne(ctx, map[string]string{"email": email}).Decode(&rec)
	if err != nil {
		if err == mongodrv.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}

	return fromRecord(rec)
}

func (r *AccountRepository) FindByUsername(ctx context.Context, username string) (*accountdomain.UserAccount, error) {
	var rec accountRecord
	err := r.col.FindOne(ctx, map[string]string{"username": username}).Decode(&rec)
	if err != nil {
		if err == mongodrv.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}

	return fromRecord(rec)
}

func (r *AccountRepository) FindAll(ctx context.Context) ([]*accountdomain.UserAccount, error) {
	cursor, err := r.col.Find(ctx, map[string]string{})
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = cursor.Close(ctx)
	}()

	accounts := make([]*accountdomain.UserAccount, 0)
	for cursor.Next(ctx) {
		var rec accountRecord
		if err := cursor.Decode(&rec); err != nil {
			return nil, err
		}
		account, err := fromRecord(rec)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return accounts, nil
}
