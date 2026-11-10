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
	// Field-level updates preserve newer/unknown fields and the profile_change
	// mirror. Only the dedicated review transaction may change that mirror.
	_, err = r.col.UpdateOne(ctx, bson.M{"_id": record.Id}, customerProfileUpdate(record), options.UpdateOne().SetUpsert(true))
	return err
}

func (r *CustomerRepository) FindByID(ctx context.Context, id string) (*accountdomain.Customer, error) {
	recordID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, nil
	}
	var record customerRecord
	if err := r.col.FindOne(ctx, bson.M{"_id": recordID}).Decode(&record); err != nil {
		if err == mongodrv.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return fromCustomerRecord(record), nil
}

func (r *CustomerRepository) FindByIdentityNumber(ctx context.Context, number string) (*accountdomain.Customer, error) {
	var record customerRecord
	err := r.col.FindOne(ctx, bson.M{"identity.number": number}).Decode(&record)
	if err != nil {
		if err == mongodrv.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return fromCustomerRecord(record), nil
}

func (r *CustomerRepository) DeleteByID(ctx context.Context, id string) error {
	recordID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	_, err = r.col.DeleteOne(ctx, bson.M{"_id": recordID})
	return err
}

func customerProfileUpdate(record customerRecord) bson.M {
	return bson.M{
		"$set": bson.M{
			"customer_code":                 record.CustomerCode,
			"profile.full_name":             record.Profile.FullName,
			"profile.date_of_birth":         record.Profile.DateOfBirth,
			"profile.phone":                 record.Profile.Phone,
			"profile.email":                 record.Profile.Email,
			"profile.address.line":          record.Profile.Address.Line,
			"profile.address.province_code": record.Profile.Address.ProvinceCode,
			"avatar":                        record.Avatar,
			"documents":                     record.Documents,
			"identity.type":                 record.Identity.Type,
			"identity.number":               record.Identity.Number,
			"identity.issued_date":          record.Identity.IssuedDate,
			"identity.issued_place":         record.Identity.IssuedPlace,
			"identity.permanent_address":    record.Identity.PermanentAddress,
			"verified_kyc_session_id":       record.VerifiedKYCSessionId,
			"kyc_status":                    record.KYCStatus,
			"credit_profile":                record.CreditProfile,
			"status":                        record.Status,
			"updated_at":                    record.UpdatedAt,
		},
		"$setOnInsert": bson.M{"created_at": record.CreatedAt},
	}
}
