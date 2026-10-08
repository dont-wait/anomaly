package eventstore

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"

	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/google/uuid"
	esdb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ImportMongo is an explicit offline cutover, not a fallback on the command
// path. One append batch imports the old snapshot/history into an EMPTY stream.
// Historical completions retain ledger snapshots without applying their debit
// a second time to imported current balances.
func (r *BankRepository) ImportMongo(ctx context.Context, client *mongodrv.Client, dbName string) error {
	accounts, err := mongorepo.NewAccountAggregateRepository(client, dbName).FindAll(ctx)
	if err != nil {
		return err
	}
	// Stable order/time make a retry of the same offline source deterministic.
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].Id < accounts[j].Id })
	records := []journal.Record{{Type: journal.BankImported, At: time.Unix(0, 0).UTC(), Accounts: accounts}}
	db := client.Database(dbName)
	cursor, err := db.Collection("transactions").Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}, {Key: "transaction_id", Value: 1}}))
	if err != nil {
		return err
	}
	defer func() { _ = cursor.Close(ctx) }()
	for cursor.Next(ctx) {
		var tx mongorepo.TransferDocument
		if err := cursor.Decode(&tx); err != nil {
			return err
		}
		if tx.Type != "transfer" {
			return fmt.Errorf("unsupported historical transaction type %q", tx.Type)
		}
		tx.IdempotencyKey = strings.ToLower(tx.IdempotencyKey)
		pending := tx
		pending.Status = "awaiting_otp"
		pending.PostedAt = time.Time{}
		created, err := transferRecord(pending, eventFor(pending, journal.Created, 1, tx.CreatedAt))
		if err != nil {
			return err
		}
		created.Historical = true
		records = append(records, *created)
		var terminal *journal.Record
		switch tx.Status {
		case "awaiting_otp":
			continue
		case "success":
			var debit, credit struct {
				Before int64 `bson:"balance_before"`
				After  int64 `bson:"balance_after"`
			}
			for _, side := range []struct {
				direction string
				id        bson.ObjectID
				target    any
			}{{"DEBIT", tx.Source.AccountID, &debit}, {"CREDIT", tx.Destination.AccountID, &credit}} {
				if err := db.Collection("ledger_entries").FindOne(ctx, bson.M{"transaction_id": tx.ID, "account_id": side.id, "direction": side.direction}).Decode(side.target); err != nil {
					return fmt.Errorf("missing historical ledger for %s: %w", tx.ID, err)
				}
			}
			e := eventFor(tx, journal.Completed, 2, tx.PostedAt)
			e.Payload.SourceBalanceBefore, e.Payload.SourceBalanceAfter = &debit.Before, &debit.After
			e.Payload.DestinationBalanceBefore, e.Payload.DestinationBalanceAfter = &credit.Before, &credit.After
			terminal, err = transferRecord(tx, e)
		case "cancelled":
			var feed struct {
				At time.Time `bson:"occurred_at"`
			}
			if err := db.Collection("account_transaction_feed").FindOne(ctx, bson.M{"transaction_id": tx.ID, "status": "cancelled"}).Decode(&feed); err != nil {
				return err
			}
			terminal, err = transferRecord(tx, eventFor(tx, journal.Cancelled, 2, feed.At))
		default:
			return fmt.Errorf("unsupported historical transfer status %q", tx.Status)
		}
		if err != nil {
			return err
		}
		terminal.Historical = true
		records = append(records, *terminal)
	}
	if err := cursor.Err(); err != nil {
		return err
	}
	return r.importRecords(ctx, records)
}

// ImportLegacyAccounts preserves the already-authoritative account-* streams.
// Unlogged Mongo transfers require explicit reconciliation rather than silently
// overriding balances from the event store.
func (r *BankRepository) ImportLegacyAccounts(ctx context.Context, client *mongodrv.Client, dbName string) error {
	count, err := client.Database(dbName).Collection("transactions").CountDocuments(ctx, bson.M{})
	if err != nil {
		return err
	}
	if count != 0 {
		return fmt.Errorf("mongo contains unlogged MVP transactions; reconcile before choosing an explicit Mongo cutover")
	}
	accounts, err := NewAccountRepository(r.client).FindAll(ctx)
	if err != nil {
		return err
	}
	if len(accounts) == 0 {
		return fmt.Errorf("no legacy account events found; a new database needs no import")
	}
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].Id < accounts[j].Id })
	return r.importRecords(ctx, []journal.Record{{Type: journal.BankImported, At: time.Unix(0, 0).UTC(), Accounts: accounts}})
}

func (r *BankRepository) importRecords(ctx context.Context, records []journal.Record) error {
	manifest, err := json.Marshal(records)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(manifest)
	records[0].ID = journal.EventID(fmt.Sprintf("%x", hash[:]), journal.BankImported)
	state := &bankState{Accounts: map[string]*accountdomain.UserAccount{}, Balances: map[string]int64{}, Transfers: map[string]transferState{}, Keys: map[string]string{}}
	events := make([]esdb.EventData, 0, len(records))
	size := 0
	for i, record := range records {
		data, err := json.Marshal(record)
		if err != nil {
			return err
		}
		if _, err := journal.DecodeRecord(data, record.ID, record.Type); err != nil {
			return err
		}
		if err := state.apply(record); err != nil {
			return err
		}
		state.Exists = true
		state.Revision = uint64(i)
		size += len(data)
		events = append(events, esdb.EventData{EventID: uuid.MustParse(record.ID), EventType: record.Type, ContentType: esdb.ContentTypeJson, Data: data})
	}
	// Stay below the server's default message limit; no partial import occurs.
	if size > 3*1024*1024 {
		return fmt.Errorf("offline import exceeds 3 MiB; a staged import is required")
	}
	existing, err := r.client.ReadStream(ctx, journal.BankStream, esdb.ReadStreamOptions{RequiresLeader: true}, 1)
	if err == nil {
		defer existing.Close()
		first, readErr := existing.Recv()
		if readErr == nil {
			if first.Event.EventType == journal.BankImported && first.Event.EventID.String() == records[0].ID {
				return nil
			}
			return fmt.Errorf("banking stream already exists; refusing to overwrite source of truth")
		}
		if !resourceMissing(readErr) {
			return readErr
		}
	} else if !resourceMissing(err) {
		return err
	}
	_, err = r.client.AppendToStream(ctx, journal.BankStream, esdb.AppendToStreamOptions{StreamState: esdb.NoStream{}}, events...)
	return err
}
