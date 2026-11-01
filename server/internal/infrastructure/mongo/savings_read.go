package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	savingsqueries "github.com/dont-wait/anomaly/internal/application/savings/queries"
	"github.com/dont-wait/anomaly/internal/domain/account"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type SavingsRead struct {
	db       *mongodrv.Database
	accounts *AccountRepository
}

func NewSavingsRead(client *mongodrv.Client, db string) *SavingsRead {
	return &SavingsRead{db: client.Database(db), accounts: NewAccountRepository(client, db)}
}

func (r *SavingsRead) FindByID(ctx context.Context, id string) (*account.UserAccount, error) {
	return r.accounts.FindByID(ctx, id)
}

func (r *SavingsRead) EnsureIndexes(ctx context.Context) error {
	if r == nil || r.db == nil {
		return savingsqueries.ErrReadNotReady
	}
	_, err := r.db.Collection("savings_contracts").Indexes().CreateMany(ctx, []mongodrv.IndexModel{
		{Keys: bson.D{{Key: "customer_id", Value: 1}, {Key: "opened_at", Value: -1}, {Key: "_id", Value: -1}}, Options: options.Index().SetName("savings_customer_opened")},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "maturity_at", Value: 1}, {Key: "maturity_notice_at", Value: 1}}, Options: options.Index().SetName("savings_due_notification")},
	})
	if err != nil {
		return err
	}
	_, err = r.db.Collection("savings_notifications").Indexes().CreateOne(ctx, mongodrv.IndexModel{Keys: bson.D{{Key: "customer_id", Value: 1}, {Key: "created_at", Value: -1}}, Options: options.Index().SetName("savings_notification_customer")})
	return err
}

func (r *SavingsRead) Snapshot(ctx context.Context) (int64, error) {
	if r == nil || r.db == nil {
		return 0, savingsqueries.ErrReadNotReady
	}
	var cp struct {
		Revision int64 `bson:"revision"`
	}
	err := r.db.Collection("checkpoints").FindOne(ctx, bson.M{"_id": bankCheckpointID}).Decode(&cp)
	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return 0, savingsqueries.ErrReadNotReady
	}
	if err != nil {
		return 0, err
	}
	if cp.Revision < 0 {
		return 0, savingsqueries.ErrReadNotReady
	}
	return cp.Revision + 1, nil
}

func (r *SavingsRead) List(ctx context.Context, customer, status string, limit int, cursor *savingsqueries.SavingsCursor, now time.Time) ([]savingsqueries.SavingsContract, error) {
	filter := bson.M{"customer_id": customer}
	switch status {
	case "active":
		filter["status"] = bson.M{"$in": bson.A{"active", "matured"}}
		filter["maturity_at"] = bson.M{"$gt": now}
	case "matured":
		filter["status"] = bson.M{"$in": bson.A{"active", "matured"}}
		filter["maturity_at"] = bson.M{"$lte": now}
	case "settled":
		filter["status"] = "settled"
	case "all":
	default:
		return nil, savingsqueries.ErrInvalidRead
	}
	if cursor != nil {
		filter["$or"] = bson.A{
			bson.M{"opened_at": bson.M{"$lt": cursor.OpenedAt}},
			bson.M{"opened_at": cursor.OpenedAt, "_id": bson.M{"$lt": cursor.ID}},
		}
	}
	opts := options.Find().SetSort(bson.D{{Key: "opened_at", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(int64(limit))
	cur, err := r.db.Collection("savings_contracts").Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()
	var rows []savingsqueries.SavingsContract
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *SavingsRead) Detail(ctx context.Context, customer, id string) (*savingsqueries.SavingsContract, error) {
	if customer == "" || id == "" {
		return nil, savingsqueries.ErrInvalidRead
	}
	var row savingsqueries.SavingsContract
	err := r.db.Collection("savings_contracts").FindOne(ctx, bson.M{"_id": id, "customer_id": customer}).Decode(&row)
	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return nil, savingsqueries.ErrContractNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *SavingsRead) History(ctx context.Context, customer, id string) ([]savingsqueries.SavingsHistoryItem, error) {
	c, err := r.Detail(ctx, customer, id)
	if err != nil {
		return nil, err
	}
	accountID, err := bson.ObjectIDFromHex(journal.FinancialAccountID(c.SourceAccountID))
	if err != nil {
		return nil, fmt.Errorf("invalid financial account id: %w", err)
	}
	filter := bson.M{"account_id": accountID, "transaction_id": bson.M{"$in": bson.A{journal.EventID(c.SavingsID, "deposit"), journal.EventID(c.SavingsID, "withdrawal")}}, "type": bson.M{"$in": bson.A{"savings_deposit", "savings_withdrawal"}}}
	cur, err := r.db.Collection("ledger_entries").Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}, {Key: "transaction_id", Value: 1}}).SetLimit(2))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()
	var records []savingsqueries.SavingsHistoryItem
	if err := cur.All(ctx, &records); err != nil {
		return nil, err
	}
	return records, nil
}

func (r *SavingsRead) Notifications(ctx context.Context, customer string, limit int) ([]savingsqueries.SavingsNotice, error) {
	// Preserve previously delivered notices in history even after settlement.
	// The customer ownership check also rejects orphan/foreign contract IDs.
	// canWithdraw is a projection-aware UI hint, NOT permission to withdraw:
	// the command checks the authoritative Banking Stream again.
	pipeline := mongodrv.Pipeline{
		{{Key: "$match", Value: bson.M{"customer_id": customer}}},
		{{Key: "$lookup", Value: bson.M{"from": "savings_contracts", "localField": "savings_id", "foreignField": "_id", "as": "contract"}}},
		{{Key: "$unwind", Value: "$contract"}},
		{{Key: "$match", Value: bson.M{"contract.customer_id": customer}}},
		{{Key: "$addFields", Value: bson.M{"can_withdraw": bson.M{"$in": bson.A{"$contract.status", bson.A{"active", "matured"}}}}}},
		{{Key: "$sort", Value: bson.D{{Key: "created_at", Value: -1}, {Key: "_id", Value: -1}}}},
		{{Key: "$limit", Value: int64(limit)}},
		{{Key: "$project", Value: bson.M{"contract": 0}}},
	}
	cur, err := r.db.Collection("savings_notifications").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()
	var items []savingsqueries.SavingsNotice
	if err := cur.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}
