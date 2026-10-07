package mongo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const bankCheckpointID = "banking-projection"

type BankProjection struct {
	client *mongodrv.Client
	db     *mongodrv.Database
}

func NewBankProjection(client *mongodrv.Client, dbName string) *BankProjection {
	return &BankProjection{client: client, db: client.Database(dbName)}
}

func (p *BankProjection) Position(ctx context.Context) (uint64, bool, error) {
	var row struct {
		Revision uint64 `bson:"revision"`
	}
	err := p.db.Collection("checkpoints").FindOne(ctx, bson.M{"_id": bankCheckpointID}).Decode(&row)
	if err == mongodrv.ErrNoDocuments {
		return 0, false, nil
	}
	return row.Revision, err == nil, err
}

func (p *BankProjection) Reset(ctx context.Context) error {
	_, err := p.db.Collection("checkpoints").DeleteOne(ctx, bson.M{"_id": bankCheckpointID})
	return err
}

// Projection and checkpoint commit together. Duplicate and older delivery never
// applies a second debit or moves the checkpoint backwards.
func (p *BankProjection) Apply(ctx context.Context, record journal.Record, revision uint64) error {
	session, err := p.client.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)
	_, err = session.WithTransaction(ctx, func(sc context.Context) (any, error) {
		last, found, err := p.Position(sc)
		if err != nil {
			return nil, err
		}
		if found && last >= revision {
			return nil, nil
		}
		if (found && revision != last+1) || (!found && revision != 0) {
			return nil, fmt.Errorf("bank projection received an out-of-order revision")
		}
		sequence := int64(revision + 1)
		switch record.Type {
		case journal.AccountCreated, journal.AccountUpdated:
			if err := p.profile(sc, record.Account); err != nil {
				return nil, err
			}
		case journal.BankImported:
			for _, a := range record.Accounts {
				if err := p.profile(sc, a); err != nil {
					return nil, err
				}
				if err := p.balance(sc, journal.FinancialAccountID(a.Id), a.Balance.Current, sequence, bankImportBalanceAt(a, record.At)); err != nil {
					return nil, err
				}
			}
		case journal.BalanceSeeded:
			if err := p.balance(sc, journal.FinancialAccountID(record.AccountID), *record.Balance, sequence, record.At); err != nil {
				return nil, err
			}
		default:
			if !journal.IsTransferEvent(record.Type) {
				return nil, fmt.Errorf("unsupported banking event")
			}
			var tx TransferDocument
			if err := json.Unmarshal(record.Transfer, &tx); err != nil {
				return nil, err
			}
			if _, err := p.db.Collection("transactions").ReplaceOne(sc, bson.M{"transaction_id": tx.ID}, tx, options.Replace().SetUpsert(true)); err != nil {
				return nil, err
			}
			e := record.Event
			if e.Type == journal.Completed {
				for _, side := range []struct {
					id            string
					direction     string
					amount        int64
					before, after int64
				}{
					{e.Payload.SourceAccountID, "DEBIT", tx.Amount + tx.Fee, *e.Payload.SourceBalanceBefore, *e.Payload.SourceBalanceAfter},
					{e.Payload.DestinationAccountID, "CREDIT", tx.Amount, *e.Payload.DestinationBalanceBefore, *e.Payload.DestinationBalanceAfter},
				} {
					accountID, err := bson.ObjectIDFromHex(side.id)
					if err != nil {
						return nil, err
					}
					if !record.Historical {
						if err := p.balance(sc, side.id, side.after, sequence, e.OccurredAt); err != nil {
							return nil, err
						}
					}
					entry := bson.M{"transaction_id": tx.ID, "account_id": accountID, "sequence": sequence, "direction": side.direction, "amount": side.amount, "balance_before": side.before, "balance_after": side.after, "created_at": e.OccurredAt}
					if _, err := p.db.Collection("ledger_entries").UpdateOne(sc, bson.M{"transaction_id": tx.ID, "direction": side.direction}, bson.M{"$set": entry}, options.UpdateOne().SetUpsert(true)); err != nil {
						return nil, err
					}
				}
				if err := p.feed(sc, tx, "IN", e.Payload.DestinationBalanceAfter, e.OccurredAt); err != nil {
					return nil, err
				}
			}
			if e.Type == journal.Completed || e.Type == journal.Cancelled {
				if err := p.feed(sc, tx, "OUT", e.Payload.SourceBalanceAfter, e.OccurredAt); err != nil {
					return nil, err
				}
			}
		}
		_, err = p.db.Collection("checkpoints").UpdateOne(sc, bson.M{"_id": bankCheckpointID}, bson.M{"$set": bson.M{"revision": revision}}, options.UpdateOne().SetUpsert(true))
		return nil, err
	})
	return err
}

func (p *BankProjection) profile(ctx context.Context, a *accountdomain.UserAccount) error {
	if a == nil || a.Customer == nil {
		return fmt.Errorf("missing account profile")
	}
	if err := NewAccountRepository(p.client, p.db.Name()).Save(ctx, a); err != nil {
		return err
	}
	if err := NewCustomerRepository(p.client, p.db.Name()).Save(ctx, a.Customer); err != nil {
		return err
	}
	for _, session := range a.KYCSessions {
		if err := NewKYCSessionRepository(p.client, p.db.Name()).Save(ctx, session); err != nil {
			return err
		}
	}
	return nil
}

func (p *BankProjection) balance(ctx context.Context, id string, balance, version int64, at time.Time) error {
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	result, err := p.db.Collection("accounts").UpdateOne(ctx, bson.M{"financial_id": objectID}, bson.M{"$set": bson.M{"balance.current": balance, "financial_version": version, "updated_at": at}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return fmt.Errorf("missing account projection for %s", id)
	}
	return nil
}

func bankImportBalanceAt(account *accountdomain.UserAccount, fallback time.Time) time.Time {
	if !account.UpdatedAt.IsZero() {
		return account.UpdatedAt
	}
	if !account.CreatedAt.IsZero() {
		return account.CreatedAt
	}
	return fallback
}

func (p *BankProjection) feed(ctx context.Context, tx TransferDocument, direction string, balance *int64, at time.Time) error {
	id, no, name, bank, fee := tx.Source.AccountID, tx.Destination.AccountNo, tx.Destination.Name, tx.Destination.BankCode, tx.Fee
	if direction == "IN" {
		id, no, name, bank, fee = tx.Destination.AccountID, tx.Source.AccountNo, tx.Source.Name, "ANOMALY", 0
	}
	row := bson.M{"account_id": id, "transaction_id": tx.ID, "direction": direction, "type": tx.Type, "counterparty": bson.M{"account_no": no, "name": name, "bank_code": bank}, "amount": tx.Amount, "fee": fee, "status": tx.Status, "note": tx.Note, "reference": tx.Reference, "search_text": normalizeFeedSearch(name + " " + no + " " + tx.Note + " " + tx.Reference + " " + fmt.Sprint(tx.Amount)), "occurred_at": at}
	if balance != nil {
		row["balance_after"] = *balance
	}
	_, err := p.db.Collection("account_transaction_feed").UpdateOne(ctx, bson.M{"account_id": id, "transaction_id": tx.ID}, bson.M{"$set": row}, options.UpdateOne().SetUpsert(true))
	return err
}
