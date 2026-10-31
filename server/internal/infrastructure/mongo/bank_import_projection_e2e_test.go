//go:build bank_import_e2e

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

// Uses a disposable replica set; NEVER uses the app's MongoDB URI.
func TestBankImportedBaselineCutoverE2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	c, err := mongodb.Run(ctx, "mongo:7.0", mongodb.WithReplicaSet("rs0"))
	if err != nil {
		t.Fatal(err)
	}
	testcontainers.CleanupContainer(t, c)
	uri, err := c.ConnectionString(ctx)
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

	db := client.Database("anomaly_cutover_test_" + uuid.NewString()[:8])
	defer db.Drop(context.Background())
	accountID := bson.NewObjectID()
	const baseline int64 = 138_540_000
	if _, err := db.Collection("accounts").InsertOne(ctx, bson.M{
		"_id": accountID, "financial_id": accountID,
		"financial_version": int64(7), "balance": bson.M{"current": baseline},
	}); err != nil {
		t.Fatal(err)
	}
	p := NewBankProjection(client, db.Name())
	at := time.Now().UTC()
	if err := p.importedBalance(ctx, accountID.Hex(), baseline, 1, at); err != nil {
		t.Fatal(err)
	}
	check := func(wantBalance, wantVersion int64) {
		t.Helper()
		var a struct {
			Balance struct {
				Current int64 `bson:"current"`
			} `bson:"balance"`
			FinancialVersion int64 `bson:"financial_version"`
			Marker           int64 `bson:"banking_import_baseline_version"`
		}
		if err := db.Collection("accounts").FindOne(ctx, bson.M{"_id": accountID}).Decode(&a); err != nil {
			t.Fatal(err)
		}
		if a.Balance.Current != wantBalance || a.FinancialVersion != wantVersion || a.Marker != 1 {
			t.Fatalf("balance=%d version=%d marker=%d; want balance=%d version=%d marker=1", a.Balance.Current, a.FinancialVersion, a.Marker, wantBalance, wantVersion)
		}
	}
	check(baseline, 1)

	// The next canonical savings debit (sequence=2) must not be skipped.
	if err := p.balance(ctx, accountID.Hex(), baseline-1_000_000, 2, at); err != nil {
		t.Fatal(err)
	}
	check(baseline-1_000_000, 2)

	// Replaying BankImported must not revert a newer canonical balance.
	if err := p.importedBalance(ctx, accountID.Hex(), baseline, 1, at); err != nil {
		t.Fatal(err)
	}
	check(baseline-1_000_000, 2)

	// A different legacy Mongo balance without the marker must fail closed.
	other := bson.NewObjectID()
	if _, err := db.Collection("accounts").InsertOne(ctx, bson.M{
		"_id": other, "financial_id": other,
		"financial_version": int64(7), "balance": bson.M{"current": baseline + 1},
	}); err != nil {
		t.Fatal(err)
	}
	if err := p.importedBalance(ctx, other.Hex(), baseline, 1, at); err == nil {
		t.Fatal("mismatched baseline must not be imported")
	}

	// Even with a marker, a changed baseline and no new banking revision is unsafe.
	corrupt := bson.NewObjectID()
	if _, err := db.Collection("accounts").InsertOne(ctx, bson.M{
		"_id": corrupt, "financial_id": corrupt,
		"financial_version": int64(1), "banking_import_baseline_version": int64(1),
		"balance": bson.M{"current": baseline + 1},
	}); err != nil {
		t.Fatal(err)
	}
	if err := p.importedBalance(ctx, corrupt.Hex(), baseline, 1, at); err == nil {
		t.Fatal("a marked baseline with an unexplained balance change must fail")
	}
}
