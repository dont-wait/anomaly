package mongo

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	txdomain "github.com/dont-wait/anomaly/internal/domain/transaction"
)

// TransactionRepository ghi transaction + feed entries. Ghi tuần tự (best-effort), không dùng Mongo session/transaction — chấp nhận được vì đây là
// read-model phụ trợ, nguồn sự thật số dư vẫn là accounts.balance.
type TransactionRepository struct {
	transactions *mongodrv.Collection
	feed         *mongodrv.Collection
}

func NewTransactionRepository(client *mongodrv.Client, dbName string) *TransactionRepository {
	db := client.Database(dbName)
	return &TransactionRepository{
		transactions: db.Collection("transactions"),
		feed:         db.Collection("account_transaction_feed"),
	}
}

func (r *TransactionRepository) Create(
	ctx context.Context,
	tx *txdomain.Transaction,
	feedEntries []*txdomain.FeedEntry,
) error {
	record, err := toTransactionRecord(tx)
	if err != nil {
		return err
	}
	if _, err := r.transactions.InsertOne(ctx, record); err != nil {
		if IsDuplicateKeyError(err) {
			return txdomain.ErrIdempotencyKeyConflict
		}
		return err
	}

	docs := make([]any, 0, len(feedEntries))
	for _, entry := range feedEntries {
		feedRec, err := toFeedRecord(entry)
		if err != nil {
			return err
		}
		docs = append(docs, feedRec)
	}
	if len(docs) > 0 {
		if _, err := r.feed.InsertMany(ctx, docs); err != nil {
			return err
		}
	}
	return nil
}

func (r *TransactionRepository) FindByAccountID(
	ctx context.Context,
	accountId string,
	limit int,
) ([]*txdomain.FeedEntry, error) {
	accID, err := bsonObjectIDFromHexOrNil(accountId)
	if err != nil || accID == nil {
		return nil, nil
	}

	opts := options.Find().
		SetSort(map[string]int{"occurred_at": -1}).
		SetLimit(int64(limit))
	cursor, err := r.feed.Find(ctx, map[string]any{"account_id": accID}, opts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	entries := make([]*txdomain.FeedEntry, 0, limit)
	for cursor.Next(ctx) {
		var rec feedRecord
		if err := cursor.Decode(&rec); err != nil {
			return nil, err
		}
		entries = append(entries, fromFeedRecord(rec))
	}
	return entries, cursor.Err()
}

func (r *TransactionRepository) FindBySourceAndIdempotencyKey(
	ctx context.Context,
	sourceAccountId, idempotencyKey string,
) (*txdomain.Transaction, error) {
	accID, err := bson.ObjectIDFromHex(sourceAccountId)
	if err != nil {
		return nil, err
	}

	var record transactionRecord
	err = r.transactions.FindOne(ctx, map[string]any{
		"source_account_id": accID,
		"idempotency_key":   idempotencyKey,
	}).Decode(&record)
	if err != nil {
		if err == mongodrv.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return fromTransactionRecord(record), nil
}
