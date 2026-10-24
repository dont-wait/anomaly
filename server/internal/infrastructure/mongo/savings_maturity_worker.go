package mongo

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/savings"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// SavingsMaturityWorker produces one durable in-app notice per matured savings
// contract. It NEVER modifies the account balance, settlement, or banking stream.
// Multiple API replicas are safe: eligibility and the deterministic insert are
// committed in a Mongo transaction (Mongo replica set required).
type SavingsMaturityWorker struct {
	Client *mongodrv.Client
	DB     *mongodrv.Database
	Now    func() time.Time
}

func NewSavingsMaturityWorker(client *mongodrv.Client, db string) *SavingsMaturityWorker {
	return &SavingsMaturityWorker{Client: client, DB: client.Database(db)}
}

func (w *SavingsMaturityWorker) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}

func (w *SavingsMaturityWorker) RunOnce(ctx context.Context) (int, error) {
	if w == nil || w.Client == nil || w.DB == nil {
		return 0, fmt.Errorf("savings worker not initialized")
	}
	now := w.now()
	filter := bson.M{"status": bson.M{"$in": bson.A{"active", "matured"}}, "maturity_at": bson.M{"$lte": now}, "maturity_notice_at": bson.M{"$exists": false}}
	cur, err := w.DB.Collection("savings_contracts").Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "maturity_at", Value: 1}}).SetLimit(100))
	if err != nil {
		return 0, err
	}
	var rows []struct {
		ID         string    `bson:"_id"`
		CustomerID string    `bson:"customer_id"`
		SavingsNo  string    `bson:"savings_no"`
		MaturityAt time.Time `bson:"maturity_at"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return 0, err
	}
	count := 0
	for _, row := range rows {
		if row.ID == "" || row.CustomerID == "" || row.MaturityAt.IsZero() {
			return count, fmt.Errorf("invalid savings maturity projection")
		}
		session, err := w.Client.StartSession()
		if err != nil {
			return count, err
		}
		result, txErr := session.WithTransaction(ctx, func(sc context.Context) (any, error) {
			// Write to the SAME contract document as the banking projector to
			// prevent a concurrent settlement from racing past eligibility.
			claim, err := w.DB.Collection("savings_contracts").UpdateOne(sc,
				bson.M{"_id": row.ID, "customer_id": row.CustomerID, "status": bson.M{"$in": bson.A{"active", "matured"}}, "maturity_at": bson.M{"$lte": now}, "maturity_notice_at": bson.M{"$exists": false}},
				bson.M{"$set": bson.M{"maturity_notice_at": now}})
			if err != nil || claim.MatchedCount == 0 {
				return false, err
			}
			notice := bson.M{"_id": "savings-maturity:" + row.ID, "customer_id": row.CustomerID, "savings_id": row.ID, "savings_no": row.SavingsNo, "type": "savings_matured", "maturity_at": row.MaturityAt, "created_at": now}
			// _id uniquely identifies one notice for the life of the contract.
			_, err = w.DB.Collection("savings_notifications").UpdateOne(sc, bson.M{"_id": notice["_id"]}, bson.M{"$setOnInsert": notice}, options.UpdateOne().SetUpsert(true))
			return err == nil, err
		})
		session.EndSession(ctx)
		if txErr != nil {
			return count, txErr
		}
		if claimed, ok := result.(bool); ok && claimed {
			count++
		}
	}
	return count, nil
}

func (w *SavingsMaturityWorker) Run(ctx context.Context) {
	tick := time.NewTicker(savings.MaturityPollInterval)
	defer tick.Stop()
	for {
		scanCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		_, err := w.RunOnce(scanCtx)
		cancel()
		if err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "savings maturity notification scan failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
