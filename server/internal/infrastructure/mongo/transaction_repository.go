package mongo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/text/unicode/norm"
)

var (
	ErrTransactionAccountNotFound = errors.New("account not found")
	ErrTransactionConflict        = errors.New("transaction conflict")
	ErrTransactionInsufficient    = errors.New("insufficient funds")
	ErrTransactionBalanceOverflow = errors.New("destination balance overflow")
)

type TransferAccount struct {
	ID        bson.ObjectID
	Customer  bson.ObjectID
	AccountNo string
	Name      string
	Email     string
	Balance   int64
	Version   int64
	Status    string
}

type TransferDocument struct {
	ID        string `bson:"transaction_id"`
	Reference string `bson:"reference"`
	Type      string `bson:"type"`
	Source    struct {
		AccountID bson.ObjectID `bson:"account_id"`
		AccountNo string        `bson:"account_no"`
		Name      string        `bson:"name"`
	} `bson:"source"`
	Destination struct {
		Type      string        `bson:"type"`
		AccountID bson.ObjectID `bson:"account_id"`
		AccountNo string        `bson:"account_no"`
		BankCode  string        `bson:"bank_code"`
		Name      string        `bson:"name"`
	} `bson:"destination"`
	Amount         int64     `bson:"amount"`
	Fee            int64     `bson:"fee"`
	Currency       string    `bson:"currency"`
	Channel        string    `bson:"channel"`
	Status         string    `bson:"status"`
	Note           string    `bson:"note"`
	IdempotencyKey string    `bson:"idempotency_key"`
	CreatedAt      time.Time `bson:"created_at"`
	PostedAt       time.Time `bson:"posted_at,omitempty"`
	Authentication struct {
		Method   string `bson:"method"`
		Verified bool   `bson:"verified"`
	} `bson:"authentication"`
	OTPExpiresAt time.Time `bson:"otp_expires_at,omitempty"`
	OTPResendAt  time.Time `bson:"otp_resend_available_at,omitempty"`
}

type FeedDocument struct {
	ID            bson.ObjectID `bson:"_id"`
	AccountID     bson.ObjectID `bson:"account_id"`
	TransactionID string        `bson:"transaction_id"`
	Direction     string        `bson:"direction"`
	Type          string        `bson:"type"`
	Counterparty  struct {
		AccountNo string `bson:"account_no"`
		Name      string `bson:"name"`
		BankCode  string `bson:"bank_code"`
	} `bson:"counterparty"`
	Amount       int64     `bson:"amount"`
	Fee          int64     `bson:"fee"`
	BalanceAfter *int64    `bson:"balance_after"`
	Status       string    `bson:"status"`
	Note         string    `bson:"note"`
	Reference    string    `bson:"reference"`
	SearchText   string    `bson:"search_text"`
	OccurredAt   time.Time `bson:"occurred_at"`
}

type TransferRepository struct {
	client *mongodrv.Client
	db     *mongodrv.Database
}

func NewTransferRepository(client *mongodrv.Client, dbName string) *TransferRepository {
	return &TransferRepository{client: client, db: client.Database(dbName)}
}

func (r *TransferRepository) FindOwnedAccount(ctx context.Context, userID string) (TransferAccount, error) {
	recordID, err := accountRecordID(userID)
	if err != nil {
		return TransferAccount{}, ErrTransactionAccountNotFound
	}
	var account accountRecord
	if err := r.db.Collection("accounts").FindOne(ctx, bson.M{"_id": recordID}).Decode(&account); err != nil {
		if errors.Is(err, mongodrv.ErrNoDocuments) {
			return TransferAccount{}, ErrTransactionAccountNotFound
		}
		return TransferAccount{}, err
	}
	var customer struct {
		Profile struct {
			FullName string `bson:"full_name"`
		} `bson:"profile"`
	}
	if err := r.db.Collection("customers").FindOne(ctx, bson.M{"_id": account.CustomerId}).Decode(&customer); err != nil {
		return TransferAccount{}, err
	}
	objectID, ok := recordID.(bson.ObjectID)
	if !ok {
		return TransferAccount{}, fmt.Errorf("account id is not an object id")
	}
	return TransferAccount{ID: objectID, Customer: account.CustomerId, AccountNo: account.AccountNo, Name: customer.Profile.FullName, Email: account.Email, Balance: account.Balance.Current, Version: account.Version, Status: account.Status}, nil
}

func (r *TransferRepository) FindRecipient(ctx context.Context, accountNo string) (TransferAccount, error) {
	var account accountRecord
	if err := r.db.Collection("accounts").FindOne(ctx, bson.M{"account_no": accountNo, "status": "active"}).Decode(&account); err != nil {
		if errors.Is(err, mongodrv.ErrNoDocuments) {
			return TransferAccount{}, ErrTransactionAccountNotFound
		}
		return TransferAccount{}, err
	}
	var id bson.ObjectID
	switch value := account.Id.(type) {
	case bson.ObjectID:
		id = value
	default:
		return TransferAccount{}, fmt.Errorf("account id is not an object id")
	}
	var customer struct {
		Profile struct {
			FullName string `bson:"full_name"`
		} `bson:"profile"`
	}
	if err := r.db.Collection("customers").FindOne(ctx, bson.M{"_id": account.CustomerId}).Decode(&customer); err != nil {
		return TransferAccount{}, err
	}
	return TransferAccount{ID: id, Customer: account.CustomerId, AccountNo: account.AccountNo, Name: customer.Profile.FullName, Email: account.Email, Balance: account.Balance.Current, Version: account.Version, Status: account.Status}, nil
}

func (r *TransferRepository) FindAccountByObjectID(ctx context.Context, id bson.ObjectID) (TransferAccount, error) {
	var account accountRecord
	if err := r.db.Collection("accounts").FindOne(ctx, bson.M{"_id": id, "status": "active"}).Decode(&account); err != nil {
		if errors.Is(err, mongodrv.ErrNoDocuments) {
			return TransferAccount{}, ErrTransactionAccountNotFound
		}
		return TransferAccount{}, err
	}
	var customer struct {
		Profile struct {
			FullName string `bson:"full_name"`
		} `bson:"profile"`
	}
	if err := r.db.Collection("customers").FindOne(ctx, bson.M{"_id": account.CustomerId}).Decode(&customer); err != nil {
		return TransferAccount{}, err
	}
	return TransferAccount{ID: id, Customer: account.CustomerId, AccountNo: account.AccountNo, Name: customer.Profile.FullName, Email: account.Email, Balance: account.Balance.Current, Version: account.Version, Status: account.Status}, nil
}

func (r *TransferRepository) ListFeed(ctx context.Context, accountID bson.ObjectID, filter bson.M, limit int64) ([]FeedDocument, error) {
	filter["account_id"] = accountID
	opts := options.Find().SetSort(bson.D{{Key: "occurred_at", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(limit)
	cursor, err := r.db.Collection("account_transaction_feed").Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	items := make([]FeedDocument, 0)
	for cursor.Next(ctx) {
		var row FeedDocument
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		items = append(items, row)
	}
	return items, cursor.Err()
}

type FeedCursor struct {
	OccurredAt time.Time `json:"occurredAt"`
	ID         string    `json:"id"`
}

func EncodeFeedCursor(cursor FeedCursor) string {
	b, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(b)
}

func DecodeFeedCursor(value string) (FeedCursor, error) {
	var cursor FeedCursor
	b, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return cursor, err
	}
	if err := json.Unmarshal(b, &cursor); err != nil {
		return cursor, err
	}
	if cursor.OccurredAt.IsZero() {
		return cursor, fmt.Errorf("invalid cursor")
	}
	if _, err := bson.ObjectIDFromHex(cursor.ID); err != nil {
		return cursor, fmt.Errorf("invalid cursor")
	}
	return cursor, nil
}

func FeedCursorFor(row FeedDocument) string {
	return EncodeFeedCursor(FeedCursor{OccurredAt: row.OccurredAt, ID: row.ID.Hex()})
}

func (r *TransferRepository) Summary(ctx context.Context, accountID bson.ObjectID, from, to time.Time) (int64, int64, error) {
	pipeline := bson.A{
		bson.M{"$match": bson.M{"account_id": accountID, "status": "success", "occurred_at": bson.M{"$gte": from, "$lt": to}}},
		bson.M{"$group": bson.M{"_id": "$direction", "total": bson.M{"$sum": "$amount"}}},
	}
	cursor, err := r.db.Collection("account_transaction_feed").Aggregate(ctx, pipeline)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	var in, out int64
	for cursor.Next(ctx) {
		var row struct {
			ID    string `bson:"_id"`
			Total int64  `bson:"total"`
		}
		if err := cursor.Decode(&row); err != nil {
			return 0, 0, err
		}
		switch row.ID {
		case "IN":
			in = row.Total
		case "OUT":
			out = row.Total
		}
	}
	return in, out, cursor.Err()
}

func (r *TransferRepository) GetFeed(ctx context.Context, accountID bson.ObjectID, transactionID string) (FeedDocument, error) {
	var row FeedDocument
	err := r.db.Collection("account_transaction_feed").FindOne(ctx, bson.M{"account_id": accountID, "transaction_id": transactionID}).Decode(&row)
	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return FeedDocument{}, ErrTransactionAccountNotFound
	}
	return row, err
}

func (r *TransferRepository) AccountObjectID(ctx context.Context, userID string) (bson.ObjectID, error) {
	a, err := r.FindOwnedAccount(ctx, userID)
	return a.ID, err
}

func (r *TransferRepository) FeedCollection() *mongodrv.Collection {
	return r.db.Collection("account_transaction_feed")
}

func normalizeFeedSearch(value string) string {
	value = strings.ReplaceAll(strings.ToLower(value), "đ", "d")
	var b strings.Builder
	for _, char := range norm.NFD.String(value) {
		if !unicode.Is(unicode.Mn, char) {
			b.WriteRune(char)
		}
	}
	return b.String()
}
