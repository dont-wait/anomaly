//go:build e2e

package eventstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/application/account/commands"
	"github.com/dont-wait/anomaly/internal/domain"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/google/uuid"
	esdb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
)

func TestBankEventSourcing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	mc, err := mongodb.Run(ctx, "mongo:7.0", mongodb.WithReplicaSet("rs0"))
	if err != nil {
		t.Fatal(err)
	}
	testcontainers.CleanupContainer(t, mc)
	uri, err := mc.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	client, err := mongorepo.NewMongoClient(ctx, &domain.MongoConfig{MongoURI: uri})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	db := client.Database("bank_event_sourcing")
	applyMigrations(t, ctx, db)
	es := startEventStore(t, ctx)
	bank := eventstore.NewBankRepository(es, mongorepo.NewTransferRepository(client, db.Name()))
	register := commands.NewRegisterAccountCommandHandler(bank, bank)
	var ids []string
	for i := range 2 {
		a, err := register.Handle(ctx, commands.RegisterAccountCommand{IdempotencyKey: uuid.NewString(), Username: fmt.Sprintf("bank-%d", i), Email: fmt.Sprintf("bank-%d@example.com", i), Password: "TestBank@123", CCCDNumber: fmt.Sprintf("%012d", i+1), CCCDIssuedDate: time.Now().Add(-time.Hour), DOB: time.Now().AddDate(-25, 0, 0)})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, a.Id)
	}
	verify := commands.NewVerifyAccountCommandHandler(bank)
	if _, err := verify.Handle(ctx, commands.VerifyAccountCommand{AccountID: ids[0], IdCardFrontUrl: "kyc/" + ids[0] + "/id-card-front/front", IdCardBackUrl: "kyc/" + ids[0] + "/id-card-back/back", LiveVideoUrl: "kyc/" + ids[0] + "/live-video/live"}); err != nil {
		t.Fatal(err)
	}
	if err := bank.SetSeedBalance(ctx, ids[0], 1000); err != nil {
		t.Fatal(err)
	}
	assertCount(t, ctx, db, "accounts", 0) // Mongo has never received any write.
	assertCount(t, ctx, db, "transactions", 0)
	source, err := bank.FindOwnedAccount(ctx, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	destination, err := bank.FindOwnedAccount(ctx, ids[1])
	if err != nil {
		t.Fatal(err)
	}
	if source.Balance != 1000 {
		t.Fatal("funds were read from Mongo rather than EventStoreDB")
	}
	lookup, err := bank.FindRecipient(ctx, destination.AccountNo)
	if err != nil || lookup.ID != destination.ID {
		t.Fatalf("recipient lookup = %+v/%v", lookup, err)
	}
	transfers := []mongorepo.TransferDocument{newTransfer(source, destination), newTransfer(source, destination)}
	for _, tx := range transfers {
		if _, created, err := bank.CreatePending(ctx, tx); err != nil || !created {
			t.Fatalf("create = %v/%v", created, err)
		}
		if err := bank.SetOTPMetadata(ctx, tx.ID, time.Now().Add(5*time.Minute), time.Now().Add(30*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	// Two independent repository instances spend the same remaining balance.
	type result struct {
		id  string
		err error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, tx := range transfers {
		wg.Go(func() {
			actor := eventstore.NewBankRepository(es, nil)
			_, err := actor.ConfirmTransfer(ctx, tx.ID, ids[0])
			results <- result{tx.ID, err}
		})
	}
	wg.Wait()
	close(results)
	var winner, loser string
	for result := range results {
		if result.err == nil {
			winner = result.id
		} else if errors.Is(result.err, mongorepo.ErrTransactionInsufficient) {
			loser = result.id
		} else {
			t.Fatal(result.err)
		}
	}
	if winner == "" || loser == "" {
		t.Fatal("expected exactly one debit and one insufficient-funds result")
	}
	for range 4 {
		if _, err := bank.ConfirmTransfer(ctx, winner, ids[0]); err != nil {
			t.Fatal(err)
		}
	}
	assertCount(t, ctx, db, "transactions", 0)
	feed, err := bank.GetFeed(ctx, source.ID, winner)
	if err != nil || feed.BalanceAfter == nil || *feed.BalanceAfter != 0 {
		t.Fatalf("canonical confirm feed = %+v/%v", feed, err)
	}
	if _, err := bank.FindTransfer(ctx, winner, ids[1]); !errors.Is(err, mongorepo.ErrTransactionAccountNotFound) {
		t.Fatal("recipient authorized to confirm sender's transfer")
	}
	for _, tx := range transfers {
		if tx.ID == winner {
			if existing, created, err := bank.CreatePending(ctx, tx); err != nil || created || existing.Status != "success" {
				t.Fatalf("idempotent create after spend = %v/%v/%v", existing.Status, created, err)
			}
		}
	}
	for range 2 {
		if err := bank.CancelPending(ctx, loser); err != nil {
			t.Fatal(err)
		}
	}
	assertBankBalance(t, ctx, eventstore.NewBankRepository(es, nil), ids[0], 0)
	assertBankBalance(t, ctx, eventstore.NewBankRepository(es, nil), ids[1], 1000)

	records := readBank(t, ctx, es)
	// Resume from the committed revision (duplicate delivery is accepted by the
	// projection); this exercises actual subscription boundary semantics.
	resumeCtx, resumeCancel := context.WithTimeout(ctx, 5*time.Second)
	subscription, err := es.SubscribeToStream(resumeCtx, journal.BankStream, esdb.SubscribeToStreamOptions{From: esdb.Revision(uint64(len(records) - 2))})
	if err != nil {
		t.Fatal(err)
	}
	resumed := false
	for !resumed {
		message := subscription.Recv()
		if message.SubscriptionDropped != nil {
			t.Fatal(message.SubscriptionDropped.Error)
		}
		if message.EventAppeared != nil {
			revision := message.EventAppeared.Event.EventNumber
			if revision < uint64(len(records)-2) || revision > uint64(len(records)-1) {
				t.Fatalf("unexpected resume revision %d", revision)
			}
			resumed = revision == uint64(len(records)-1)
		}
	}
	_ = subscription.Close()
	resumeCancel()

	projection := mongorepo.NewBankProjection(client, db.Name())
	var completedIndex int
	for i, record := range records {
		if record.Type == journal.Completed {
			completedIndex = i
			break
		}
		if err := projection.Apply(ctx, record, uint64(i)); err != nil {
			t.Fatal(err)
		}
	}
	assertProjectedBalance(t, ctx, db, source.ID, 1000)
	// Fail a feed write AFTER account and ledger updates. None may commit.
	if err := db.RunCommand(ctx, bson.D{{Key: "collMod", Value: "account_transaction_feed"}, {Key: "validator", Value: bson.M{"amount": bson.M{"$gt": 1000000}}}}).Err(); err != nil {
		t.Fatal(err)
	}
	if err := projection.Apply(ctx, records[completedIndex], uint64(completedIndex)); err == nil {
		t.Fatal("invalid feed did not fail projection")
	}
	assertProjectedBalance(t, ctx, db, source.ID, 1000)
	assertCount(t, ctx, db, "ledger_entries", 0)
	position, found, err := projection.Position(ctx)
	if err != nil || !found || position != uint64(completedIndex-1) {
		t.Fatalf("rollback checkpoint = %d/%v/%v", position, found, err)
	}
	restoreFeedValidator(t, ctx, db)
	for i := completedIndex; i < len(records); i++ {
		if err := projection.Apply(ctx, records[i], uint64(i)); err != nil {
			t.Fatal(err)
		}
	}
	for i, record := range records {
		if err := projection.Apply(ctx, record, uint64(i)); err != nil {
			t.Fatal(err)
		}
	}
	assertCount(t, ctx, db, "ledger_entries", 2)
	assertCount(t, ctx, db, "account_transaction_feed", 3)
	var posted mongorepo.TransferDocument
	if err := db.Collection("transactions").FindOne(ctx, bson.M{"transaction_id": winner}).Decode(&posted); err != nil || !posted.Authentication.Verified || posted.Authentication.Method != "email_otp" {
		t.Fatalf("authentication projection = %+v/%v", posted.Authentication, err)
	}

	assertCount(t, ctx, db, "outbox_events", 0)
	assertProjectedBalance(t, ctx, db, source.ID, 0)
	assertProjectedBalance(t, ctx, db, destination.ID, 1000)

	// Delete the entire business read model and rebuild it from the journal.
	for _, name := range []string{"accounts", "customers", "kyc_sessions", "transactions", "ledger_entries", "account_transaction_feed"} {
		if _, err := db.Collection(name).DeleteMany(ctx, bson.M{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := projection.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	for i, record := range records {
		if err := projection.Apply(ctx, record, uint64(i)); err != nil {
			t.Fatal(err)
		}
	}
	assertCount(t, ctx, db, "accounts", 2)
	assertCount(t, ctx, db, "transactions", 2)
	assertCount(t, ctx, db, "ledger_entries", 2)
	assertCount(t, ctx, db, "account_transaction_feed", 3)
	assertProjectedBalance(t, ctx, db, source.ID, 0)
	assertProjectedBalance(t, ctx, db, destination.ID, 1000)
	a, err := mongorepo.NewAccountAggregateRepository(client, db.Name()).FindByID(ctx, ids[0])
	if err != nil || a == nil || !a.IsVerified() {
		t.Fatalf("KYC replay = %+v/%v", a, err)
	}

	// Explicit offline import works into an empty canonical stream and does not
	// double-apply historical debits to the imported current balances.
	importES := startEventStore(t, ctx)
	imported := eventstore.NewBankRepository(importES, nil)
	for range 2 {
		if err := imported.ImportMongo(ctx, client, db.Name()); err != nil {
			t.Fatal(err)
		}
	}
	assertBankBalance(t, ctx, imported, ids[0], 0)
	assertBankBalance(t, ctx, imported, ids[1], 1000)
	if err := bank.ImportMongo(ctx, client, db.Name()); err == nil {
		t.Fatal("import overwrote an existing canonical stream")
	}
	reverse := newTransfer(destination, source)
	if _, _, err := imported.CreatePending(ctx, reverse); err != nil {
		t.Fatal(err)
	}
	if _, err := imported.ConfirmTransfer(ctx, reverse.ID, ids[1]); err != nil {
		t.Fatal(err)
	}
	assertBankBalance(t, ctx, imported, ids[0], 1000)
	assertBankBalance(t, ctx, imported, ids[1], 0)

	// Preserve existing authoritative legacy account streams, even when the
	// Mongo projection is empty. Startup must not fork a fresh canonical history.
	legacyES := startEventStore(t, ctx)
	legacyAccount, err := bank.FindByID(ctx, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	legacyAccount.Balance.Current = 5000
	if err := eventstore.NewAccountRepository(legacyES).Save(ctx, legacyAccount); err != nil {
		t.Fatal(err)
	}
	legacyBank := eventstore.NewBankRepository(legacyES, nil)
	if err := legacyBank.EnsureReady(ctx, false); err == nil {
		t.Fatal("startup ignored canonical legacy account streams")
	}
	for range 2 {
		if err := legacyBank.ImportLegacyAccounts(ctx, client, "legacy_migration_empty_projection"); err != nil {
			t.Fatal(err)
		}
	}
	if err := legacyBank.EnsureReady(ctx, false); err != nil {
		t.Fatal(err)
	}
	assertBankBalance(t, ctx, legacyBank, ids[0], 5000)

	// EventStore failure must never fall back to Mongo writes.
	badES, err := eventstore.NewEventStoreClient(&domain.EventStoreConfig{EventStoreConnString: "esdb://127.0.0.1:1?tls=false"})
	if err != nil {
		t.Fatal(err)
	}
	defer eventstore.Disconnect(badES)
	failureCtx, failureCancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer failureCancel()
	if _, _, err := eventstore.NewBankRepository(badES, mongorepo.NewTransferRepository(client, db.Name())).CreatePending(failureCtx, newTransfer(source, destination)); err == nil {
		t.Fatal("command succeeded without EventStoreDB")
	}
	assertCount(t, ctx, db, "transactions", 2)
}

func startEventStore(t *testing.T, ctx context.Context) *esdb.Client {
	t.Helper()
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: testcontainers.ContainerRequest{Image: "eventstore/eventstore:23.10.8-bookworm-slim", ExposedPorts: []string{"2113/tcp"}, Env: map[string]string{"EVENTSTORE_INSECURE": "true", "EVENTSTORE_RUN_PROJECTIONS": "None"}, WaitingFor: wait.ForHTTP("/health/live").WithPort("2113/tcp").WithStatusCodeMatcher(func(status int) bool { return status == 204 || status == 200 }).WithStartupTimeout(90 * time.Second)}, Started: true})
	if err != nil {
		t.Fatal(err)
	}
	testcontainers.CleanupContainer(t, c)
	host, err := c.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := c.MappedPort(ctx, "2113/tcp")
	if err != nil {
		t.Fatal(err)
	}
	client, err := eventstore.NewEventStoreClient(&domain.EventStoreConfig{EventStoreConnString: fmt.Sprintf("esdb://%s:%s?tls=false", host, port.Port())})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { eventstore.Disconnect(client) })
	return client
}

func readBank(t *testing.T, ctx context.Context, client *esdb.Client) []journal.Record {
	t.Helper()
	stream, err := client.ReadStream(ctx, journal.BankStream, esdb.ReadStreamOptions{}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var records []journal.Record
	for {
		e, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		r, err := journal.DecodeRecord(e.Event.Data, e.Event.EventID.String(), e.Event.EventType)
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, r)
	}
	return records
}

func newTransfer(source, destination mongorepo.TransferAccount) mongorepo.TransferDocument {
	id := uuid.NewString()
	tx := mongorepo.TransferDocument{ID: id, Reference: "FT" + id, Type: "transfer", Amount: 1000, Currency: "VND", Channel: "web", Status: "awaiting_otp", IdempotencyKey: uuid.NewString(), CreatedAt: time.Now().UTC()}
	tx.Source.AccountID, tx.Source.AccountNo, tx.Source.Name = source.ID, source.AccountNo, source.Name
	tx.Destination.Type, tx.Destination.AccountID, tx.Destination.AccountNo, tx.Destination.Name, tx.Destination.BankCode = "internal", destination.ID, destination.AccountNo, destination.Name, "ANOMALY"
	return tx
}

func assertCount(t *testing.T, ctx context.Context, db *mongodrv.Database, collection string, want int64) {
	t.Helper()
	got, err := db.Collection(collection).CountDocuments(ctx, bson.M{})
	if err != nil || got != want {
		t.Fatalf("%s count = %d/%v; want %d", collection, got, err, want)
	}
}

func assertBankBalance(t *testing.T, ctx context.Context, bank *eventstore.BankRepository, id string, want int64) {
	t.Helper()
	a, err := bank.FindByID(ctx, id)
	if err != nil || a == nil || a.Balance.Current != want {
		t.Fatalf("canonical balance = %+v/%v; want %d", a, err, want)
	}
}

func assertProjectedBalance(t *testing.T, ctx context.Context, db *mongodrv.Database, id bson.ObjectID, want int64) {
	t.Helper()
	var a struct {
		Balance struct {
			Current int64 `bson:"current"`
		} `bson:"balance"`
	}
	err := db.Collection("accounts").FindOne(ctx, bson.M{"financial_id": id}).Decode(&a)
	if err != nil || a.Balance.Current != want {
		t.Fatalf("projected balance = %d/%v; want %d", a.Balance.Current, err, want)
	}
}

func applyMigrations(t *testing.T, ctx context.Context, db *mongodrv.Database) {
	t.Helper()
	files, err := filepath.Glob("../../../migrations/*.up.json")
	if err != nil || len(files) == 0 {
		t.Fatal("no migrations")
	}
	for _, file := range files {
		runMigration(t, ctx, db, file, "")
	}
}

func restoreFeedValidator(t *testing.T, ctx context.Context, db *mongodrv.Database) {
	t.Helper()
	runMigration(t, ctx, db, "../../../migrations/000005_create_transaction_collections.up.json", "account_transaction_feed")
}

func runMigration(t *testing.T, ctx context.Context, db *mongodrv.Database, file, onlyCollection string) {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var commands []json.RawMessage
	if err := json.Unmarshal(data, &commands); err != nil {
		t.Fatal(err)
	}
	for _, raw := range commands {
		var command bson.D
		if err := bson.UnmarshalExtJSON(raw, false, &command); err != nil {
			t.Fatal(err)
		}
		if onlyCollection != "" && (command[0].Key != "collMod" || command[0].Value != onlyCollection) {
			continue
		}
		if err := db.RunCommand(ctx, command).Err(); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
	}
}
