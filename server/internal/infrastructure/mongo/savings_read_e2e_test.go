//go:build savings_e2e

package mongo

import (
	"context"
	"fmt"
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
type savingsMaturityTestVerifier struct {
	settled     map[string]bool
	unavailable map[string]bool
}

func (v *savingsMaturityTestVerifier) SavingsMaturityEligible(ctx context.Context, customer, id string) (bool, error) {
	if v.unavailable[id] {
		return false, fmt.Errorf("test-only bank stream unavailable")
	}
	return !v.settled[id], nil
}

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
	verifier := &savingsMaturityTestVerifier{settled: map[string]bool{}, unavailable: map[string]bool{}}
	worker := NewSavingsMaturityWorker(client, db.Name(), verifier)
	worker.Now = func() time.Time { return now }
	if n, err := worker.RunOnce(ctx); err != nil || n != 2 {
		t.Fatalf("first maturity scan: %d / %v", n, err)
	} // owner and outsider; not future/settled
	if n, err := worker.RunOnce(ctx); err != nil || n != 0 {
		t.Fatalf("duplicate notification: %d / %v", n, err)
	}
	notifications, err := read.Notifications(ctx, "owner", 50)
	if err != nil || len(notifications) != 1 || notifications[0].SavingsID != "mine" || !notifications[0].CanWithdraw {
		t.Fatalf("owner notification must allow matured withdrawal: %+v / %v", notifications, err)
	}
	// A delivered notification is history, not a pending-only queue.
	// It remains readable after settlement but its withdrawal action stops.
	if _, err := db.Collection("savings_contracts").UpdateOne(ctx, bson.M{"_id": "mine"}, bson.M{"$set": bson.M{"status": "settled", "settlement": bson.M{"transaction_id": "tx-mine", "settled_at": now}}}); err != nil {
		t.Fatal(err)
	}
	historical, err := read.Notifications(ctx, "owner", 50)
	if err != nil || len(historical) != 1 || historical[0].SavingsID != "mine" || historical[0].CanWithdraw {
		t.Fatalf("settled contract must retain notice without withdrawal action: %+v / %v", historical, err)
	}
	// A forged notice claiming the user's ID but pointing to another user's
	// contract must never appear in the notification history.
	if _, err := db.Collection("savings_notifications").InsertOne(ctx, bson.M{
		"_id": "savings-maturity:spoofed", "customer_id": "owner", "savings_id": "outsider",
		"type": "savings_matured", "created_at": now,
	}); err != nil {
		t.Fatal(err)
	}
	if owned, err := read.Notifications(ctx, "owner", 50); err != nil || len(owned) != 1 {
		t.Fatalf("foreign contract notice leaked to customer: %+v / %v", owned, err)
	}
	if _, err := db.Collection("savings_contracts").UpdateOne(ctx, bson.M{"_id": "future"}, bson.M{"$set": bson.M{"status": "settled", "settlement": bson.M{"transaction_id": "tx2", "interest_amount": int64(1), "total_amount": int64(1_000_001), "settled_at": now}}}); err != nil {
		t.Fatal(err)
	}
	worker.Now = func() time.Time { return later.Add(time.Minute) }
	if n, err := worker.RunOnce(ctx); err != nil || n != 0 {
		t.Fatalf("settled contract produced a notice: %d / %v", n, err)
	}

	// Regression: >100 malformed due rows used to block the same first 100
	// forever. Both decoded-invalid rows and BSON decode errors must be skipped.
	bad := make([]any, 0, 106)
	for i := 0; i < 105; i++ {
		row := bson.M{"_id": fmt.Sprintf("bad-%03d", i), "status": "active", "maturity_at": due.Add(-time.Hour), "savings_no": "SVBAD"}
		if i%2 == 0 {
			row["customer_id"] = int32(42) // malformed BSON type
		} // odd records have no customer ID
		bad = append(bad, row)
	}
	if _, err := db.Collection("savings_contracts").InsertMany(ctx, bad); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Collection("savings_contracts").InsertOne(ctx, bson.M{
		"_id": "after-invalid-rows", "customer_id": "owner", "savings_no": "SVTAIL",
		"status": "active", "maturity_at": due, "opened_at": due.AddDate(0, -3, 0),
	}); err != nil {
		t.Fatal(err)
	}
	if n, err := worker.RunOnce(ctx); err == nil || n != 1 {
		t.Fatalf("bad records must be reported without starving a valid row: %d / %v", n, err)
	}
	notices, err := read.Notifications(ctx, "owner", 50)
	if err != nil || len(notices) != 2 {
		t.Fatalf("later valid notice was not persisted: %+v / %v", notices, err)
	}
	for _, notice := range notices {
		if notice.SavingsID == "mine" && notice.CanWithdraw {
			t.Fatal("settled historical notice regained an available withdrawal action")
		}
	}
	for i := 0; i < 105; i++ {
		if _, err := db.Collection("savings_contracts").DeleteOne(ctx, bson.M{"_id": fmt.Sprintf("bad-%03d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := worker.RunOnce(ctx); err != nil || n != 0 {
		t.Fatalf("post-cleanup should not duplicate notices: %d / %v", n, err)
	}

	// A lagging Mongo projection still calls a contract active even though
	// the authoritative stream reports it settled. No notice may be claimed.
	verifier.settled["already-settled-in-eventstore"] = true
	if _, err := db.Collection("savings_contracts").InsertOne(ctx, bson.M{
		"_id": "already-settled-in-eventstore", "customer_id": "owner", "savings_no": "SVLAG",
		"status": "active", "maturity_at": due,
	}); err != nil {
		t.Fatal(err)
	}
	if n, err := worker.RunOnce(ctx); err != nil || n != 0 {
		t.Fatalf("stale Mongo projection generated notice: %d / %v", n, err)
	}
	if err := db.Collection("savings_notifications").FindOne(ctx, bson.M{"_id": "savings-maturity:already-settled-in-eventstore"}).Err(); err == nil {
		t.Fatal("notification was inserted for an already settled contract")
	}
	// A failed authoritative read must also fail closed.
	verifier.unavailable["unverified"] = true
	if _, err := db.Collection("savings_contracts").InsertOne(ctx, bson.M{
		"_id": "unverified", "customer_id": "owner", "savings_no": "SVUNVERIFIED",
		"status": "active", "maturity_at": due,
	}); err != nil {
		t.Fatal(err)
	}
	if n, err := worker.RunOnce(ctx); err == nil || n != 0 {
		t.Fatalf("unavailable authoritative stream must fail closed: %d / %v", n, err)
	}
	if _, err := db.Collection("savings_contracts").DeleteMany(ctx, bson.M{"_id": bson.M{"$in": bson.A{"unverified", "already-settled-in-eventstore"}}}); err != nil {
		t.Fatal(err)
	}

	// Force one notification transaction to fail validation. The worker must
	// process the next due row and leave the failed contract unclaimed for retry.
	if err := db.RunCommand(ctx, bson.D{
		{Key: "collMod", Value: "savings_notifications"},
		{Key: "validator", Value: bson.M{"savings_no": bson.M{"$ne": "SVFAIL"}}},
	}).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Collection("savings_contracts").InsertMany(ctx, []any{
		bson.M{"_id": "failure", "customer_id": "owner", "savings_no": "SVFAIL", "status": "active", "maturity_at": due.Add(-30 * time.Minute)},
		bson.M{"_id": "success", "customer_id": "owner", "savings_no": "SVSUCCESS", "status": "active", "maturity_at": due.Add(-10 * time.Minute)},
	}); err != nil {
		t.Fatal(err)
	}
	if n, err := worker.RunOnce(ctx); err == nil || n != 1 {
		t.Fatalf("transaction failure blocked later contract: %d / %v", n, err)
	}
	var failedDoc bson.M
	if err := db.Collection("savings_contracts").FindOne(ctx, bson.M{"_id": "failure"}).Decode(&failedDoc); err != nil {
		t.Fatal(err)
	}
	if _, claimed := failedDoc["maturity_notice_at"]; claimed {
		t.Fatalf("failed transaction incorrectly claimed notification: %+v", failedDoc)
	}
	if err := db.Collection("savings_notifications").FindOne(ctx, bson.M{"_id": "savings-maturity:success"}).Err(); err != nil {
		t.Fatalf("later valid notification missing after transaction failure: %v", err)
	}
}
