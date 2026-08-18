package mongo

import (
	"context"

	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

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

func (r *AccountRepository) Save(ctx context.Context, a *accountdomain.UserAccount) error {
	_, err := r.col.ReplaceOne(
		ctx,
		map[string]string{"_id": a.Id},
		toRecord(a),
		options.Replace().SetUpsert(true),
	)
	return err
}

func (r *AccountRepository) FindByID(ctx context.Context, id string) (*accountdomain.UserAccount, error) {
	var rec accountRecord
	err := r.col.FindOne(ctx, map[string]string{"_id": id}).Decode(&rec)
	if err != nil {
		if err == mongodrv.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}

	return fromRecord(rec), nil
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

	return fromRecord(rec), nil
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
		accounts = append(accounts, fromRecord(rec))
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return accounts, nil
}
