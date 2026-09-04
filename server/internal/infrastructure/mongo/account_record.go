package mongo

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type balanceRecord struct {
	Current bson.Decimal128 `bson:"current"`
}

type accountRecord struct {
	Id           any           `bson:"_id"`
	AccountNo    string        `bson:"account_no"`
	CustomerId   bson.ObjectID `bson:"customer_id"`
	Username     string        `bson:"username"`
	Email        string        `bson:"email"`
	PasswordHash string        `bson:"password_hash"`
	Type         string        `bson:"type"`
	Currency     string        `bson:"currency"`
	Balance      balanceRecord `bson:"balance"`
	Status       string        `bson:"status"`
	Version      int64         `bson:"version"`
	OpenedAt     time.Time     `bson:"opened_at"`
	CreatedAt    time.Time     `bson:"created_at"`
	UpdatedAt    time.Time     `bson:"updated_at"`
}

func toRecord(a *accountdomain.UserAccount) (accountRecord, error) {
	id, err := accountRecordID(a.Id)
	if err != nil {
		return accountRecord{}, fmt.Errorf("invalid account id %q: %w", a.Id, err)
	}
	customerID, err := bson.ObjectIDFromHex(a.CustomerId)
	if err != nil {
		return accountRecord{}, fmt.Errorf("invalid customer id %q: %w", a.CustomerId, err)
	}
	balance, err := bson.ParseDecimal128(strconv.FormatInt(a.Balance.Current, 10))
	if err != nil {
		return accountRecord{}, fmt.Errorf("encode account balance: %w", err)
	}

	return accountRecord{
		Id:           id,
		AccountNo:    a.AccountNo,
		CustomerId:   customerID,
		Username:     a.Username,
		Email:        a.Email,
		PasswordHash: a.PasswordHash,
		Type:         string(a.Type),
		Currency:     string(a.Currency),
		Balance:      balanceRecord{Current: balance},
		Status:       string(a.Status),
		Version:      a.Version,
		OpenedAt:     a.OpenedAt,
		CreatedAt:    a.CreatedAt,
		UpdatedAt:    a.UpdatedAt,
	}, nil
}

func fromRecord(r accountRecord) (*accountdomain.UserAccount, error) {
	id, err := accountIDFromRecord(r.Id)
	if err != nil {
		return nil, err
	}
	balance, err := strconv.ParseInt(r.Balance.Current.String(), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("decode account balance %q: %w", r.Balance.Current.String(), err)
	}

	return &accountdomain.UserAccount{
		Id:           id,
		AccountNo:    r.AccountNo,
		CustomerId:   r.CustomerId.Hex(),
		Username:     r.Username,
		Email:        r.Email,
		PasswordHash: r.PasswordHash,
		Type:         accountdomain.AccountType(r.Type),
		Currency:     accountdomain.Currency(r.Currency),
		Balance:      accountdomain.Balance{Current: balance},
		Status:       accountdomain.AccountStatus(r.Status),
		Version:      r.Version,
		OpenedAt:     r.OpenedAt,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}, nil
}

func accountRecordID(id string) (any, error) {
	if objectID, err := bson.ObjectIDFromHex(id); err == nil {
		return objectID, nil
	}
	if len(id) == 32 {
		if _, err := hex.DecodeString(id); err == nil {
			return id, nil
		}
	}
	return nil, fmt.Errorf("must be a 24- or 32-character hex string")
}

func accountIDFromRecord(id any) (string, error) {
	switch value := id.(type) {
	case bson.ObjectID:
		return value.Hex(), nil
	case string:
		if _, err := accountRecordID(value); err == nil {
			return value, nil
		}
	}
	return "", fmt.Errorf("invalid stored account id %v", id)
}
