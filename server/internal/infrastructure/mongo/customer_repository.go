package mongo

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type CustomerRepository struct {
	col *mongodrv.Collection
}

func NewCustomerRepository(client *mongodrv.Client, dbName string) *CustomerRepository {
	return &CustomerRepository{col: client.Database(dbName).Collection("customers")}
}

func (r *CustomerRepository) EnsureIndexes(ctx context.Context) error {
	indexes := []mongodrv.IndexModel{
		{
			Keys:    bson.D{{Key: "customer_code", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uniq_customer_code"),
		},
		{
			Keys: bson.D{{Key: "profile.phone", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uniq_customer_phone").
				SetPartialFilterExpression(bson.M{"profile.phone": bson.M{"$type": "string"}}),
		},
		{
			Keys: bson.D{{Key: "identity.number", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uniq_identity_number").
				SetPartialFilterExpression(bson.M{"identity.number": bson.M{"$type": "string"}}),
		},
	}
	_, err := r.col.Indexes().CreateMany(ctx, indexes)
	return err
}

func (r *CustomerRepository) Save(ctx context.Context, customer *accountdomain.Customer) error {
	record, err := toCustomerRecord(customer)
	if err != nil {
		return err
	}
	_, err = r.col.ReplaceOne(ctx, bson.M{"_id": record.Id}, record, options.Replace().SetUpsert(true))
	return err
}
