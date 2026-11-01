package mongo

import (
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type addressRecord struct {
	Line         string `bson:"line"`
	ProvinceCode string `bson:"province_code"`
}

type customerProfileRecord struct {
	FullName    string        `bson:"full_name"`
	DateOfBirth *time.Time    `bson:"date_of_birth"`
	Phone       *string       `bson:"phone"`
	Email       string        `bson:"email"`
	Address     addressRecord `bson:"address"`
}

type profileMediaRecord struct {
	ID          string    `bson:"id"`
	Type        string    `bson:"type"`
	FileName    string    `bson:"file_name"`
	ContentType string    `bson:"content_type"`
	StorageKey  string    `bson:"storage_key"`
	SizeBytes   int64     `bson:"size_bytes"`
	UploadedAt  time.Time `bson:"uploaded_at"`
	Status      string    `bson:"status"`
}

type customerIdentityRecord struct {
	Type             *string    `bson:"type"`
	Number           *string    `bson:"number"`
	IssuedDate       *time.Time `bson:"issued_date"`
	IssuedPlace      *string    `bson:"issued_place"`
	PermanentAddress *string    `bson:"permanent_address"`
}

type creditProfileRecord struct {
	Score     int       `bson:"score"`
	BadDebt   bool      `bson:"bad_debt"`
	UpdatedAt time.Time `bson:"updated_at"`
}

type customerRecord struct {
	Id                   bson.ObjectID          `bson:"_id"`
	CustomerCode         string                 `bson:"customer_code"`
	Profile              customerProfileRecord  `bson:"profile"`
	Avatar               *profileMediaRecord    `bson:"avatar,omitempty"`
	Documents            []profileMediaRecord   `bson:"documents,omitempty"`
	Identity             customerIdentityRecord `bson:"identity"`
	VerifiedKYCSessionId *bson.ObjectID         `bson:"verified_kyc_session_id"`
	KYCStatus            string                 `bson:"kyc_status"`
	CreditProfile        creditProfileRecord    `bson:"credit_profile"`
	Status               string                 `bson:"status"`
	CreatedAt            time.Time              `bson:"created_at"`
	UpdatedAt            time.Time              `bson:"updated_at"`
}

func toCustomerRecord(customer *accountdomain.Customer) (customerRecord, error) {
	id, err := bson.ObjectIDFromHex(customer.Id)
	if err != nil {
		return customerRecord{}, fmt.Errorf("invalid customer id %q: %w", customer.Id, err)
	}

	var verifiedSessionID *bson.ObjectID
	if customer.VerifiedKYCSessionId != "" {
		id, err := bson.ObjectIDFromHex(customer.VerifiedKYCSessionId)
		if err != nil {
			return customerRecord{}, fmt.Errorf("invalid verified KYC session id %q: %w", customer.VerifiedKYCSessionId, err)
		}
		verifiedSessionID = &id
	}

	var avatar *profileMediaRecord
	if customer.Avatar != nil {
		converted := toProfileMediaRecord(*customer.Avatar)
		avatar = &converted
	}
	documents := make([]profileMediaRecord, 0, len(customer.Documents))
	for _, document := range customer.Documents {
		documents = append(documents, toProfileMediaRecord(document))
	}
	return customerRecord{
		Id:           id,
		CustomerCode: customer.CustomerCode,
		Profile: customerProfileRecord{
			FullName:    customer.Profile.FullName,
			DateOfBirth: customer.Profile.DateOfBirth,
			Phone:       optionalString(customer.Profile.Phone),
			Email:       customer.Profile.Email,
			Address: addressRecord{
				Line:         customer.Profile.Address.Line,
				ProvinceCode: customer.Profile.Address.ProvinceCode,
			},
		},
		Avatar:    avatar,
		Documents: documents,
		Identity: customerIdentityRecord{
			Type:             optionalString(customer.Identity.Type),
			Number:           optionalString(customer.Identity.Number),
			IssuedDate:       customer.Identity.IssuedDate,
			IssuedPlace:      optionalString(customer.Identity.IssuedPlace),
			PermanentAddress: optionalString(customer.Identity.PermanentAddress),
		},
		VerifiedKYCSessionId: verifiedSessionID,
		KYCStatus:            string(customer.KYCStatus),
		CreditProfile: creditProfileRecord{
			Score:     customer.CreditProfile.Score,
			BadDebt:   customer.CreditProfile.BadDebt,
			UpdatedAt: customer.CreditProfile.UpdatedAt,
		},
		Status:    string(customer.Status),
		CreatedAt: customer.CreatedAt,
		UpdatedAt: customer.UpdatedAt,
	}, nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func fromCustomerRecord(record customerRecord) *accountdomain.Customer {
	var avatar *accountdomain.ProfileMedia
	if record.Avatar != nil {
		converted := fromProfileMediaRecord(*record.Avatar)
		avatar = &converted
	}
	documents := make([]accountdomain.ProfileMedia, 0, len(record.Documents))
	for _, document := range record.Documents {
		documents = append(documents, fromProfileMediaRecord(document))
	}
	return &accountdomain.Customer{
		Id:           record.Id.Hex(),
		CustomerCode: record.CustomerCode,
		Profile: accountdomain.CustomerProfile{
			FullName:    record.Profile.FullName,
			DateOfBirth: record.Profile.DateOfBirth,
			Phone:       stringValue(record.Profile.Phone),
			Email:       record.Profile.Email,
			Address: accountdomain.Address{
				Line:         record.Profile.Address.Line,
				ProvinceCode: record.Profile.Address.ProvinceCode,
			},
		},
		Avatar:    avatar,
		Documents: documents,
		Identity: accountdomain.CustomerIdentity{
			Type:             stringValue(record.Identity.Type),
			Number:           stringValue(record.Identity.Number),
			IssuedDate:       record.Identity.IssuedDate,
			IssuedPlace:      stringValue(record.Identity.IssuedPlace),
			PermanentAddress: stringValue(record.Identity.PermanentAddress),
		},
		VerifiedKYCSessionId: objectIDValue(record.VerifiedKYCSessionId),
		KYCStatus:            accountdomain.KYCStatus(record.KYCStatus),
		CreditProfile: accountdomain.CreditProfile{
			Score:     record.CreditProfile.Score,
			BadDebt:   record.CreditProfile.BadDebt,
			UpdatedAt: record.CreditProfile.UpdatedAt,
		},
		Status:    accountdomain.CustomerStatus(record.Status),
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}
}

func toProfileMediaRecord(media accountdomain.ProfileMedia) profileMediaRecord {
	return profileMediaRecord{ID: media.ID, Type: media.Type, FileName: media.FileName, ContentType: media.ContentType, StorageKey: media.StorageKey, SizeBytes: media.SizeBytes, UploadedAt: media.UploadedAt, Status: media.Status}
}

func fromProfileMediaRecord(media profileMediaRecord) accountdomain.ProfileMedia {
	return accountdomain.ProfileMedia{ID: media.ID, Type: media.Type, FileName: media.FileName, ContentType: media.ContentType, StorageKey: media.StorageKey, SizeBytes: media.SizeBytes, UploadedAt: media.UploadedAt, Status: media.Status}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func objectIDValue(value *bson.ObjectID) string {
	if value == nil {
		return ""
	}
	return value.Hex()
}
