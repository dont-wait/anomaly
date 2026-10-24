//go:build savings_e2e

package mongo

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Uses a disposable Mongo replica set, not the user's existing project data.
func TestSavingsReadAndMaturityNoticesWithMongo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	container, err := mongodb.Run(ctx, "mongo:7.0", mongodb.WithReplicaSet("rs0"))
	if err != nil {
		t.Fatal(err)
	}
	testcontainers.CleanupContainer(t, container)
	uri, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	q.Set("directConnection", "true")
	parsed.RawQuery = q.Encode()
	client, err := mongodrv.Connect(options.Client().ApplyURI(parsed.String()))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(context.Background())
	db := client.Database("savings_read_e2e_" + uuid.NewString()[:8])
	defer db.Drop(context.Background())
	if err := db.CreateCollection(ctx, "savings_contracts"); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateCollection(ctx, "checkpoints"); err != nil {
		t.Fatal(err)
	}
	read := NewSavingsRead(client, db.Name())
	if err := read.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2027, 10, 24, 15, 0, 0, 0, time.UTC)
	due := now.Add(-time.Minute)
	later := now.Add(time.Hour)
	rows := []any{
		bson.M{"_id": "mine", "savings_no": "SVMINE", "customer_id": "owner", "principal": int64(1_000_000), "package_id": "savings_standard", "source_account_id": "source", "term_months": 3, "annual_rate_bps": int64(450), "opened_at": due.AddDate(0, -3, 0), "maturity_at": due, "status": "active", "financial_revision": int64(1)},
		bson.M{"_id": "future", "savings_no": "SVFUTURE", "customer_id": "owner", "principal": int64(1_000_000), "package_id": "savings_standard", "source_account_id": "source", "term_months": 3, "annual_rate_bps": int64(450), "opened_at": later.AddDate(0, -3, 0), "maturity_at": later, "status": "active", "financial_revision": int64(1)},
		bson.M{"_id": "outsider", "savings_no": "SVOTHER", "customer_id": "someone_else", "principal": int64(1_000_000), "package_id": "savings_standard", "source_account_id": "source", "term_months": 3, "annual_rate_bps": int64(450), "opened_at": due.AddDate(0, -3, 0), "maturity_at": due, "status": "active", "financial_revision": int64(1)},
		bson.M{"_id": "settled", "savings_no": "SVSETTLED", "customer_id": "owner", "principal": int64(1_000_000), "package_id": "savings_standard", "source_account_id": "source", "term_months": 3, "annual_rate_bps": int64(450), "opened_at": due.AddDate(0, -3, 0), "maturity_at": due, "status": "settled", "financial_revision": int64(2), "settlement": bson.M{"transaction_id": "tx", "interest_amount": int64(1), "total_amount": int64(1_000_001), "settled_at": now}},
	}
	if _, err := db.Collection("savings_contracts").InsertMany(ctx, rows); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Collection("checkpoints").InsertOne(ctx, bson.M{"_id": bankCheckpointID, "revision": int64(3)}); err != nil {
		t.Fatal(err)
	}
	matured, err := read.List(ctx, "owner", "matured", 50, nil, now)
	if err != nil || len(matured) != 1 || matured[0].SavingsID != "mine" {
		t.Fatalf("wrong matured ownership results: %+v err %v", matured, err)
	}
	if _, err := read.Detail(ctx, "owner", "outsider"); err == nil {
		t.Fatal("foreign contract leaked")
	}
	worker := NewSavingsMaturityWorker(client, db.Name())
	worker.Now = func() time.Time { return now }
	if n, err := worker.RunOnce(ctx); err != nil || n != 2 {
		t.Fatalf("first maturity scan: %d / %v", n, err)
	} // owner and outsider; not future/settled
	if n, err := worker.RunOnce(ctx); err != nil || n != 0 {
		t.Fatalf("duplicate notification: %d / %v", n, err)
	}
	notifications, err := read.Notifications(ctx, "owner", 50)
	if err != nil || len(notifications) != 1 || notifications[0].SavingsID != "mine" {
		t.Fatalf("owner notification: %+v / %v", notifications, err)
	}
	if _, err := db.Collection("savings_contracts").UpdateOne(ctx, bson.M{"_id": "future"}, bson.M{"$set": bson.M{"status": "settled", "settlement": bson.M{"transaction_id": "tx2", "interest_amount": int64(1), "total_amount": int64(1_000_001), "settled_at": now}}}); err != nil {
		t.Fatal(err)
	}
	worker.Now = func() time.Time { return later.Add(time.Minute) }
	if n, err := worker.RunOnce(ctx); err != nil || n != 0 {
		t.Fatalf("settled contract produced a notice: %d / %v", n, err)
	}
}
