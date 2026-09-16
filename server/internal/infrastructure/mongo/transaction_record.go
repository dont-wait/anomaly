package mongo

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	txdomain "github.com/dont-wait/anomaly/internal/domain/transaction"
)

type transactionRecord struct {
	Id              bson.ObjectID `bson:"_id"`
	TransactionNo   string        `bson:"transaction_no"`
	Type            string        `bson:"type"`
	SourceAccountId bson.ObjectID `bson:"source_account_id"`
	SourceAccountNo string        `bson:"source_account_no"`
	DestAccountId   bson.ObjectID `bson:"dest_account_id"`
	DestAccountNo   string        `bson:"dest_account_no"`
	Amount          int64         `bson:"amount"`
	Currency        string        `bson:"currency"`
	Status          string        `bson:"status"`
	CreatedAt       time.Time     `bson:"created_at"`
	PostedAt        time.Time     `bson:"posted_at"`
}

func toTransactionRecord(t *txdomain.Transaction) (transactionRecord, error) {
	id, err := bson.ObjectIDFromHex(t.Id)
	if err != nil {
		return transactionRecord{}, err
	}
	sourceID, err := bson.ObjectIDFromHex(t.SourceAccountId)
	if err != nil {
		return transactionRecord{}, err
	}
	destID, err := bson.ObjectIDFromHex(t.DestAccountId)
	if err != nil {
		return transactionRecord{}, err
	}
	return transactionRecord{
		Id: id, TransactionNo: t.TransactionNo, Type: string(t.Type),
		SourceAccountId: sourceID, SourceAccountNo: t.SourceAccountNo,
		DestAccountId: destID, DestAccountNo: t.DestAccountNo,
		Amount: t.Amount, Currency: t.Currency, Status: string(t.Status),
		CreatedAt: t.CreatedAt, PostedAt: t.PostedAt,
	}, nil
}

type feedCounterparty struct {
	AccountNo string `bson:"account_no"`
	Name      string `bson:"name"`
}

type feedRecord struct {
	Id            bson.ObjectID    `bson:"_id"`
	AccountId     bson.ObjectID    `bson:"account_id"`
	TransactionId bson.ObjectID    `bson:"transaction_id"`
	Direction     string           `bson:"direction"`
	Type          string           `bson:"type"`
	Counterparty  feedCounterparty `bson:"counterparty"`
	Amount        int64            `bson:"amount"`
	BalanceAfter  int64            `bson:"balance_after"`
	OccurredAt    time.Time        `bson:"occurred_at"`
}

func toFeedRecord(f *txdomain.FeedEntry) (feedRecord, error) {
	id, err := bson.ObjectIDFromHex(f.Id)
	if err != nil {
		return feedRecord{}, err
	}
	accID, err := bson.ObjectIDFromHex(f.AccountId)
	if err != nil {
		return feedRecord{}, err
	}
	txID, err := bson.ObjectIDFromHex(f.TransactionId)
	if err != nil {
		return feedRecord{}, err
	}
	return feedRecord{
		Id: id, AccountId: accID, TransactionId: txID,
		Direction: string(f.Direction), Type: string(f.Type),
		Counterparty: feedCounterparty{AccountNo: f.CounterpartyNo, Name: f.CounterpartyName},
		Amount:       f.Amount, BalanceAfter: f.BalanceAfter, OccurredAt: f.OccurredAt,
	}, nil
}

func fromFeedRecord(r feedRecord) *txdomain.FeedEntry {
	return &txdomain.FeedEntry{
		Id: r.Id.Hex(), AccountId: r.AccountId.Hex(), TransactionId: r.TransactionId.Hex(),
		Direction: txdomain.FeedDirection(r.Direction), Type: txdomain.TransactionType(r.Type),
		CounterpartyName: r.Counterparty.Name, CounterpartyNo: r.Counterparty.AccountNo,
		Amount: r.Amount, BalanceAfter: r.BalanceAfter, OccurredAt: r.OccurredAt,
	}
}

func bsonObjectIDFromHexOrNil(id string) (*bson.ObjectID, error) {
	objID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}
	return &objID, nil
}
