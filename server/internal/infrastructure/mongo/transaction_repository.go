package mongo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
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
	OTPExpiresAt   time.Time `bson:"otp_expires_at,omitempty"`
	OTPResendAt    time.Time `bson:"otp_resend_available_at,omitempty"`
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

func (r *TransferRepository) CreatePending(ctx context.Context, tx TransferDocument) (TransferDocument, bool, error) {
	col := r.db.Collection("transactions")
	var existing TransferDocument
	err := col.FindOne(ctx, bson.M{"idempotency_key": tx.IdempotencyKey}).Decode(&existing)
	if err == nil {
		if existing.Source.AccountID != tx.Source.AccountID || existing.Destination.AccountNo != tx.Destination.AccountNo || existing.Amount != tx.Amount || existing.Note != tx.Note {
			return TransferDocument{}, false, ErrTransactionConflict
		}
		return existing, false, nil
	}
	if !errors.Is(err, mongodrv.ErrNoDocuments) {
		return TransferDocument{}, false, err
	}
	if _, err := col.InsertOne(ctx, tx); err != nil {
		if IsDuplicateKeyError(err) {
			if retryErr := col.FindOne(ctx, bson.M{"idempotency_key": tx.IdempotencyKey}).Decode(&existing); retryErr == nil {
				if existing.Source.AccountID == tx.Source.AccountID && existing.Destination.AccountNo == tx.Destination.AccountNo && existing.Amount == tx.Amount && existing.Note == tx.Note {
					return existing, false, nil
				}
			}
			return TransferDocument{}, false, ErrTransactionConflict
		}
		return TransferDocument{}, false, err
	}
	return tx, true, nil
}

func (r *TransferRepository) SetOTPMetadata(ctx context.Context, id string, expiresAt, resendAt time.Time) error {
	_, err := r.db.Collection("transactions").UpdateOne(ctx, bson.M{"transaction_id": id, "status": "awaiting_otp"}, bson.M{"$set": bson.M{"otp_expires_at": expiresAt, "otp_resend_available_at": resendAt}})
	return err
}

func (r *TransferRepository) CancelPending(ctx context.Context, id string) error {
	session, err := r.client.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)
	_, err = session.WithTransaction(ctx, func(sc context.Context) (any, error) {
		var tx TransferDocument
		if err := r.db.Collection("transactions").FindOne(sc, bson.M{"transaction_id": id}).Decode(&tx); err != nil {
			if errors.Is(err, mongodrv.ErrNoDocuments) {
				return nil, nil
			}
			return nil, err
		}
		if tx.Status != "awaiting_otp" {
			return nil, nil
		}
		at := time.Now().UTC()
		if _, err := r.db.Collection("transactions").UpdateOne(sc, bson.M{"transaction_id": id, "status": "awaiting_otp"}, bson.M{"$set": bson.M{"status": "cancelled"}}); err != nil {
			return nil, err
		}
		search := normalizeFeedSearch(tx.Destination.Name + " " + tx.Destination.AccountNo + " " + tx.Note + " " + tx.Reference + " " + fmt.Sprint(tx.Amount))
		feed := bson.M{"account_id": tx.Source.AccountID, "transaction_id": tx.ID, "direction": "OUT", "type": "transfer", "counterparty": bson.M{"account_no": tx.Destination.AccountNo, "name": tx.Destination.Name, "bank_code": tx.Destination.BankCode}, "amount": tx.Amount, "fee": tx.Fee, "status": "cancelled", "note": tx.Note, "reference": tx.Reference, "search_text": search, "occurred_at": at}
		_, err := r.db.Collection("account_transaction_feed").InsertOne(sc, feed)
		return nil, err
	})
	return err
}

func (r *TransferRepository) FindTransfer(ctx context.Context, transferID, ownerID string) (TransferDocument, error) {
	owner, err := accountRecordID(ownerID)
	if err != nil {
		return TransferDocument{}, ErrTransactionAccountNotFound
	}
	var tx TransferDocument
	err = r.db.Collection("transactions").FindOne(ctx, bson.M{"transaction_id": transferID, "source.account_id": owner}).Decode(&tx)
	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return TransferDocument{}, ErrTransactionAccountNotFound
	}
	return tx, err
}

func (r *TransferRepository) ConfirmTransfer(ctx context.Context, transferID, ownerID string) (TransferDocument, error) {
	owner, err := accountRecordID(ownerID)
	if err != nil {
		return TransferDocument{}, ErrTransactionAccountNotFound
	}
	session, err := r.client.StartSession()
	if err != nil {
		return TransferDocument{}, err
	}
	defer session.EndSession(ctx)
	result, err := session.WithTransaction(ctx, func(sc context.Context) (any, error) {
		var tx TransferDocument
		if err := r.db.Collection("transactions").FindOne(sc, bson.M{"transaction_id": transferID, "source.account_id": owner}).Decode(&tx); err != nil {
			if errors.Is(err, mongodrv.ErrNoDocuments) {
				return nil, ErrTransactionAccountNotFound
			}
			return nil, err
		}
		if tx.Status == "success" {
			return tx, nil
		}
		if tx.Status != "awaiting_otp" {
			return nil, ErrTransactionConflict
		}

		source, err := r.readBalance(sc, tx.Source.AccountID)
		if err != nil {
			return nil, err
		}
		destination, err := r.FindAccountByObjectID(sc, tx.Destination.AccountID)
		if err != nil {
			return nil, err
		}
		if tx.Amount > math.MaxInt64-tx.Fee || destination.Balance > math.MaxInt64-tx.Amount {
			return nil, ErrTransactionBalanceOverflow
		}
		if source.Balance < tx.Amount+tx.Fee {
			return nil, ErrTransactionInsufficient
		}
		now := time.Now().UTC()
		newSource := source.Balance - tx.Amount - tx.Fee
		newDestination := destination.Balance + tx.Amount
		sourceUpdate, err := r.db.Collection("accounts").UpdateOne(sc, bson.M{"_id": tx.Source.AccountID, "version": source.Version}, bson.M{"$set": bson.M{"balance.current": newSource, "updated_at": now}, "$inc": bson.M{"version": int64(1)}})
		if err != nil {
			return nil, err
		}
		if sourceUpdate.MatchedCount != 1 {
			return nil, ErrTransactionConflict
		}
		destinationUpdate, err := r.db.Collection("accounts").UpdateOne(sc, bson.M{"_id": tx.Destination.AccountID, "version": destination.Version}, bson.M{"$set": bson.M{"balance.current": newDestination, "updated_at": now}, "$inc": bson.M{"version": int64(1)}})
		if err != nil {
			return nil, err
		}
		if destinationUpdate.MatchedCount != 1 {
			return nil, ErrTransactionConflict
		}
		ledger := r.db.Collection("ledger_entries")
		if _, err := ledger.InsertOne(sc, bson.M{"transaction_id": tx.ID, "account_id": tx.Source.AccountID, "sequence": source.Version + 1, "direction": "DEBIT", "amount": tx.Amount + tx.Fee, "balance_before": source.Balance, "balance_after": newSource, "created_at": now}); err != nil {
			return nil, err
		}
		if _, err := ledger.InsertOne(sc, bson.M{"transaction_id": tx.ID, "account_id": tx.Destination.AccountID, "sequence": destination.Version + 1, "direction": "CREDIT", "amount": tx.Amount, "balance_before": destination.Balance, "balance_after": newDestination, "created_at": now}); err != nil {
			return nil, err
		}
		tx.Status = "success"
		tx.PostedAt = now
		if _, err := r.db.Collection("transactions").UpdateOne(sc, bson.M{"transaction_id": tx.ID, "status": "awaiting_otp"}, bson.M{"$set": bson.M{"status": "success", "posted_at": now, "authentication.method": "email_otp", "authentication.verified": true}}); err != nil {
			return nil, err
		}
		if err := r.insertFeed(sc, tx, destination, newSource, newDestination, now); err != nil {
			return nil, err
		}
		_, err = r.db.Collection("outbox_events").InsertOne(sc, bson.M{"event_id": tx.ID, "aggregate_type": "transaction", "aggregate_id": tx.ID, "event_type": "TransferCompleted", "payload": bson.M{"transaction_id": tx.ID, "source_account_id": tx.Source.AccountID, "destination_account_id": tx.Destination.AccountID, "amount": tx.Amount, "fee": tx.Fee}, "status": "PENDING", "attempt_count": int64(0), "created_at": now})
		if err != nil {
			return nil, err
		}
		return tx, nil
	})
	if err != nil {
		return TransferDocument{}, err
	}
	tx, ok := result.(TransferDocument)
	if !ok {
		return TransferDocument{}, fmt.Errorf("unexpected transfer transaction result")
	}
	return tx, nil
}

type balanceSnapshot struct {
	Balance int64
	Version int64
}

func (r *TransferRepository) readBalance(ctx context.Context, accountID bson.ObjectID) (balanceSnapshot, error) {
	var row struct {
		Balance struct {
			Current int64 `bson:"current"`
		} `bson:"balance"`
		Version int64 `bson:"version"`
	}
	if err := r.db.Collection("accounts").FindOne(ctx, bson.M{"_id": accountID, "status": "active"}).Decode(&row); err != nil {
		return balanceSnapshot{}, err
	}
	return balanceSnapshot{Balance: row.Balance.Current, Version: row.Version}, nil
}

func (r *TransferRepository) insertFeed(ctx context.Context, tx TransferDocument, destination TransferAccount, sourceBalance, destinationBalance int64, at time.Time) error {
	feed := r.db.Collection("account_transaction_feed")
	outSearch := normalizeFeedSearch(tx.Destination.Name + " " + tx.Destination.AccountNo + " " + tx.Note + " " + tx.Reference + " " + fmt.Sprint(tx.Amount))
	inSearch := normalizeFeedSearch(tx.Source.Name + " " + tx.Source.AccountNo + " " + tx.Note + " " + tx.Reference + " " + fmt.Sprint(tx.Amount))
	out := bson.M{"account_id": tx.Source.AccountID, "transaction_id": tx.ID, "direction": "OUT", "type": "transfer", "counterparty": bson.M{"account_no": tx.Destination.AccountNo, "name": tx.Destination.Name, "bank_code": tx.Destination.BankCode}, "amount": tx.Amount, "fee": tx.Fee, "balance_after": sourceBalance, "status": "success", "note": tx.Note, "reference": tx.Reference, "search_text": outSearch, "occurred_at": at}
	in := bson.M{"account_id": destination.ID, "transaction_id": tx.ID, "direction": "IN", "type": "transfer", "counterparty": bson.M{"account_no": tx.Source.AccountNo, "name": tx.Source.Name, "bank_code": "ANOMALY"}, "amount": tx.Amount, "fee": int64(0), "balance_after": destinationBalance, "status": "success", "note": tx.Note, "reference": tx.Reference, "search_text": inSearch, "occurred_at": at}
	if _, err := feed.InsertOne(ctx, out); err != nil {
		return err
	}
	_, err := feed.InsertOne(ctx, in)
	return err
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
	value = strings.ToLower(strings.ReplaceAll(value, "đ", "d"))
	var b strings.Builder
	for _, char := range norm.NFD.String(value) {
		if !unicode.Is(unicode.Mn, char) {
			b.WriteRune(char)
		}
	}
	return b.String()
}
