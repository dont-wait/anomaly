package mongo

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type KYCSessionRepository struct {
	col *mongodrv.Collection
}

func NewKYCSessionRepository(client *mongodrv.Client, dbName string) *KYCSessionRepository {
	return &KYCSessionRepository{col: client.Database(dbName).Collection("kyc_sessions")}
}

func (r *KYCSessionRepository) EnsureIndexes(ctx context.Context) error {
	indexes := []mongodrv.IndexModel{
		{
			Keys:    bson.D{{Key: "customer_id", Value: 1}, {Key: "attempt_no", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uniq_customer_attempt"),
		},
		{
			Keys:    bson.D{{Key: "customer_id", Value: 1}, {Key: "created_at", Value: -1}},
			Options: options.Index().SetName("idx_customer_created_at"),
		},
	}
	_, err := r.col.Indexes().CreateMany(ctx, indexes)
	return err
}

func (r *KYCSessionRepository) Save(ctx context.Context, session *accountdomain.KYCSession) error {
	record, err := toKYCSessionRecord(session)
	if err != nil {
		return err
	}
	_, err = r.col.ReplaceOne(ctx, bson.M{"_id": record.Id}, record, options.Replace().SetUpsert(true))
	return err
}
