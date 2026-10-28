//go:build savings_e2e

package mongo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/savings"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Starts a disposable Mongo replica set and runs the production migrations.
// Does not connect to any existing/production MongoDB instance.
func TestSavingsProjectionReplayWithMongoValidator(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
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
	// Use the mapped localhost port rather than the container's replica-set host.
	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("directConnection", "true")
	parsed.RawQuery = query.Encode()
	client, err := mongodrv.Connect(options.Client().ApplyURI(parsed.String()))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(context.Background())

	db := client.Database("anomaly_savings_e2e_" + uuid.NewString()[:8])
	defer db.Drop(context.Background())
	migrateSavingsTestDB(t, ctx, db)
	// The collection must exist before the projection's Mongo transaction.
	if err := db.CreateCollection(ctx, "savings_contracts"); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateCollection(ctx, "checkpoints"); err != nil {
		t.Fatal(err)
	}

	accountID := bson.NewObjectID()
	customerID := bson.NewObjectID()
	fid, err := bson.ObjectIDFromHex(journal.FinancialAccountID(accountID.Hex()))
	if err != nil {
		t.Fatal(err)
	}
	openedAt := time.Date(2026, time.October, 24, 9, 0, 0, 0, time.UTC)
	before := int64(5_000_000)
	if _, err := db.Collection("accounts").InsertOne(ctx, bson.M{
		"_id": accountID, "account_no": "000000123456", "customer_id": customerID,
		"username": "savings-e2e", "email": "savings-e2e@example.test", "password_hash": "test-only",
		"role": "user", "type": "payment", "currency": "VND", "status": "active",
		"balance": bson.M{"current": before}, "version": int64(1),
		"financial_id": fid, "financial_version": int64(0),
		"opened_at": openedAt, "created_at": openedAt, "updated_at": openedAt,
	}); err != nil {
		t.Fatal(err)
	}

	contract, err := savings.NewContract(savings.OpenContractParams{
		ID: uuid.NewString(), SavingsNo: "SVTEST123", CustomerID: customerID.Hex(),
		PackageID: "savings_standard", SourceAccountID: accountID.Hex(), Principal: 1_000_000,
		TermMonths: 3, AnnualRateBps: 450, OpenedAt: openedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	openedMovement := journal.SavingsMovement{
		Contract: *contract, ActorID: accountID.Hex(),
		OpeningQuoteID: uuid.NewString(), OpeningKey: accountID.Hex() + ":opening",
		TransactionID: journal.EventID(contract.ID, "deposit"),
	}
	openAfter := before - contract.Principal
	open := journal.Record{
		ID: journal.EventID(contract.ID, journal.SavingsOpened), Type: journal.SavingsOpened,
		At: openedAt, AccountID: accountID.Hex(), BalanceBefore: &before, BalanceAfter: &openAfter, Savings: &openedMovement,
	}
	projector := NewBankProjection(client, db.Name())
	if err := projector.Apply(ctx, open, 0); err != nil {
		t.Fatalf("project opening with validator: %v", err)
	}

	// A notice delivered before settlement must survive as read-only history.
	// The settlement projection must not delete it or make it actionable.
	if _, err := db.Collection("savings_notifications").InsertOne(ctx, bson.M{
		"_id": "savings-maturity:" + contract.ID, "customer_id": customerID.Hex(),
		"savings_id": contract.ID, "savings_no": contract.SavingsNo,
		"type": "savings_matured", "maturity_at": contract.MaturityAt,
		"created_at": contract.MaturityAt,
	}); err != nil {
		t.Fatal(err)
	}
	settledAt := contract.MaturityAt.Add(time.Minute)
	contract.Revision = 0
	intent, err := contract.PrepareWithdrawal(settledAt)
	if err != nil {
		t.Fatal(err)
	}
	txID := journal.EventID(contract.ID, "withdrawal")
	if err := contract.ConfirmSettled(intent, txID, 1, settledAt); err != nil {
		t.Fatal(err)
	}
	settledMovement := openedMovement
	settledMovement.Contract = *contract
	settledMovement.TransactionID = txID
	settledMovement.WithdrawalKey = accountID.Hex() + ":withdrawal"
	settledMovement.WithdrawalQuoteID = uuid.NewString()
	settleAfter := openAfter + intent.TotalAmount
	settled := journal.Record{
		ID: journal.EventID(contract.ID, journal.SavingsSettled), Type: journal.SavingsSettled,
		At: settledAt, AccountID: accountID.Hex(), BalanceBefore: &openAfter, BalanceAfter: &settleAfter, Savings: &settledMovement,
	}
	if err := projector.Apply(ctx, settled, 1); err != nil {
		t.Fatalf("project settlement with validator: %v", err)
	}
	assertSavingsProjected(t, ctx, db, contract.ID, fid, settleAfter, 2)
	assertSavingsHistoricalNotice(t, ctx, client, db, customerID.Hex(), contract.ID)

	// Replay after a checkpoint reset, with the newer settled read model still
	// present. Applying the older opening event must not resurrect Active.
	if err := projector.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	if err := projector.Apply(ctx, open, 0); err != nil {
		t.Fatalf("replay stale opening: %v", err)
	}
	assertSavingsProjected(t, ctx, db, contract.ID, fid, settleAfter, 2)
	if err := projector.Apply(ctx, settled, 1); err != nil {
		t.Fatalf("replay settlement: %v", err)
	}
	assertSavingsProjected(t, ctx, db, contract.ID, fid, settleAfter, 2)
	assertSavingsHistoricalNotice(t, ctx, client, db, customerID.Hex(), contract.ID)
}

func assertSavingsHistoricalNotice(t *testing.T, ctx context.Context, client *mongodrv.Client, db *mongodrv.Database, customerID, contractID string) {
	t.Helper()
	if n, err := db.Collection("savings_notifications").CountDocuments(ctx, bson.M{"savings_id": contractID}); err != nil || n != 1 {
		t.Fatalf("settlement/replay must retain exactly one historical notice: n=%d err=%v", n, err)
	}
	read := NewSavingsRead(client, db.Name())
	notices, err := read.Notifications(ctx, customerID, 20)
	if err != nil || len(notices) != 1 || notices[0].SavingsID != contractID || notices[0].CanWithdraw {
		t.Fatalf("historical notice exposed an invalid withdrawal action: %+v / %v", notices, err)
	}
}

func assertSavingsProjected(t *testing.T, ctx context.Context, db *mongodrv.Database, id string, fid bson.ObjectID, wantBalance int64, wantVersion int64) {
	t.Helper()
	var contract struct {
		Status     savings.ContractStatus `bson:"status"`
		Revision   int64                  `bson:"financial_revision"`
		Settlement struct {
			TransactionID string `bson:"transaction_id"`
		} `bson:"settlement"`
	}
	if err := db.Collection("savings_contracts").FindOne(ctx, bson.M{"_id": id}).Decode(&contract); err != nil {
		t.Fatal(err)
	}
	if contract.Status != savings.ContractSettled || contract.Revision != wantVersion || contract.Settlement.TransactionID != journal.EventID(id, "withdrawal") {
		t.Fatalf("savings contract reverted during replay: %+v", contract)
	}
	var account struct {
		Balance struct {
			Current int64 `bson:"current"`
		} `bson:"balance"`
	}
	if err := db.Collection("accounts").FindOne(ctx, bson.M{"financial_id": fid}).Decode(&account); err != nil || account.Balance.Current != wantBalance {
		t.Fatalf("balance=%d, err=%v, want=%d", account.Balance.Current, err, wantBalance)
	}
	for _, collection := range []string{"ledger_entries", "account_transaction_feed"} {
		count, err := db.Collection(collection).CountDocuments(ctx, bson.M{"account_id": fid})
		if err != nil || count != 2 {
			t.Fatalf("%s entries=%d, err=%v, want=2", collection, count, err)
		}
	}
}

func migrateSavingsTestDB(t *testing.T, ctx context.Context, db *mongodrv.Database) {
	t.Helper()
	files, err := filepath.Glob("../../../migrations/*.up.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations: %v", err)
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var commands []json.RawMessage
		if err := json.Unmarshal(data, &commands); err != nil {
			t.Fatal(err)
		}
		for _, command := range commands {
			var bsonCommand bson.D
			if err := bson.UnmarshalExtJSON(command, false, &bsonCommand); err != nil {
				t.Fatal(err)
			}
			if err := db.RunCommand(ctx, bsonCommand).Err(); err != nil {
				t.Fatal(fmt.Errorf("migration %s: %w", filepath.Base(file), err))
			}
		}
	}
}
