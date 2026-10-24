package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	savingsdomain "github.com/dont-wait/anomaly/internal/domain/savings"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const savingsFeedType = "savings" // supported by existing strict Mongo feed validators

// Keep savings balances, read records and ledger entries in the same Mongo
// projection transaction/checkpoint as every other banking event. EventStoreDB
// remains canonical; this data can always be rebuilt.
func (p *BankProjection) savingsMovement(ctx context.Context, record journal.Record, sequence int64) error {
	m := record.Savings
	if m == nil || record.BalanceBefore == nil || record.BalanceAfter == nil {
		return fmt.Errorf("invalid savings projection")
	}
	c := m.Contract
	if record.Type != journal.SavingsOpened && record.Type != journal.SavingsSettled {
		return fmt.Errorf("unsupported savings projection event")
	}
	if _, err := savingsContractRow(c, *m, sequence, record.At); err != nil {
		return err
	}
	financialID := journal.FinancialAccountID(c.SourceAccountID)
	if err := p.balance(ctx, financialID, *record.BalanceAfter, sequence, record.At); err != nil {
		return err
	}
	accountID, err := bson.ObjectIDFromHex(financialID)
	if err != nil {
		return err
	}
	direction, kind, amount := "DEBIT", "savings_deposit", c.Principal
	if record.Type == journal.SavingsSettled {
		direction, kind, amount = "CREDIT", "savings_withdrawal", c.Settlement.TotalAmount
	}
	entry := bson.M{"transaction_id": m.TransactionID, "account_id": accountID, "sequence": sequence, "direction": direction, "type": kind, "amount": amount, "balance_before": *record.BalanceBefore, "balance_after": *record.BalanceAfter, "created_at": record.At}
	_, err = p.db.Collection("ledger_entries").UpdateOne(ctx, bson.M{"transaction_id": m.TransactionID, "direction": direction}, bson.M{"$set": entry}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		return err
	}
	feed := bson.M{"account_id": accountID, "transaction_id": m.TransactionID, "direction": map[bool]string{true: "IN", false: "OUT"}[record.Type == journal.SavingsSettled], "type": savingsFeedType, "amount": amount, "fee": int64(0), "status": "success", "note": kind, "reference": c.SavingsNo, "counterparty": bson.M{"account_no": "", "name": "ANOMALY SAVINGS", "bank_code": "ANOMALY"}, "balance_after": *record.BalanceAfter, "occurred_at": record.At, "search_text": kind + " " + c.SavingsNo}
	_, err = p.db.Collection("account_transaction_feed").UpdateOne(ctx, bson.M{"account_id": accountID, "transaction_id": m.TransactionID}, bson.M{"$set": feed}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		return err
	}
	return p.saveSavingsContract(ctx, c, *m, sequence, record.At)
}

// savingsContractRow is the complete projection document, not a partial patch.
// Replacing the document prevents an old settlement field from surviving an
// active snapshot during a replay/backfill. See docs/savings-projection-schema.md.
func savingsContractRow(c savingsdomain.Contract, m journal.SavingsMovement, sequence int64, at time.Time) (bson.M, error) {
	if c.ID == "" || sequence <= 0 || c.Status == savingsdomain.ContractSettled && c.Settlement == nil || c.Status != savingsdomain.ContractSettled && c.Settlement != nil {
		return nil, fmt.Errorf("inconsistent savings contract projection")
	}
	row := bson.M{"_id": c.ID, "savings_no": c.SavingsNo, "customer_id": c.CustomerID, "package_id": c.PackageID, "source_account_id": c.SourceAccountID, "principal": c.Principal, "term_months": c.TermMonths, "annual_rate_bps": c.AnnualRateBps, "opened_at": c.OpenedAt, "maturity_at": c.MaturityAt, "status": c.Status, "opening_quote_id": m.OpeningQuoteID, "opening_idempotency_key": m.OpeningKey, "updated_at": at, "financial_revision": sequence}
	if c.Settlement != nil {
		row["settlement"] = bson.M{"transaction_id": c.Settlement.TransactionID, "interest_amount": c.Settlement.InterestAmount, "total_amount": c.Settlement.TotalAmount, "settled_at": c.Settlement.SettledAt}
	}
	return row, nil
}

// saveSavingsContract applies only increasing banking revisions. Resetting a
// projector checkpoint without clearing a newer read document must NEVER
// move a settled contract back to active or erase its settlement receipt.
// Projection and checkpoint are committed in the same Mongo transaction.
func (p *BankProjection) saveSavingsContract(ctx context.Context, c savingsdomain.Contract, m journal.SavingsMovement, sequence int64, at time.Time) error {
	row, err := savingsContractRow(c, m, sequence, at)
	if err != nil {
		return err
	}
	collection := p.db.Collection("savings_contracts")
	filter := bson.M{"_id": c.ID, "$or": bson.A{
		bson.M{"financial_revision": bson.M{"$lt": sequence}},
		bson.M{"financial_revision": bson.M{"$exists": false}},
	}}
	result, err := collection.ReplaceOne(ctx, filter, row)
	if err != nil {
		return err
	}
	if result.MatchedCount != 0 {
		return nil
	}
	var existing struct {
		Revision int64 `bson:"financial_revision"`
	}
	err = collection.FindOne(ctx, bson.M{"_id": c.ID}).Decode(&existing)
	if err == nil {
		if existing.Revision >= sequence {
			return nil // earlier event replayed after a checkpoint reset
		}
		return fmt.Errorf("savings contract %s revision changed while projecting: current=%d incoming=%d", c.ID, existing.Revision, sequence)
	}
	if !errors.Is(err, mongodrv.ErrNoDocuments) {
		return err
	}
	_, err = collection.InsertOne(ctx, row)
	return err // a concurrent insert must abort/retry the projection transaction
}
