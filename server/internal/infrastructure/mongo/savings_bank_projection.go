package mongo

import (
	"context"
	"fmt"

	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Keep savings balances, read records and ledger entries in the same Mongo
// projection transaction/checkpoint as every other banking event. EventStoreDB
// remains canonical; this data can always be rebuilt.
func (p *BankProjection) savingsMovement(ctx context.Context, record journal.Record, sequence int64) error {
	m := record.Savings
	if m == nil || record.BalanceBefore == nil || record.BalanceAfter == nil {
		return fmt.Errorf("invalid savings projection")
	}
	c := m.Contract
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
	feed := bson.M{"account_id": accountID, "transaction_id": m.TransactionID, "direction": map[bool]string{true: "IN", false: "OUT"}[record.Type == journal.SavingsSettled], "type": kind, "amount": amount, "fee": int64(0), "status": "success", "note": kind, "reference": c.SavingsNo, "counterparty": bson.M{"account_no": "", "name": "ANOMALY SAVINGS", "bank_code": "ANOMALY"}, "balance_after": *record.BalanceAfter, "occurred_at": record.At, "search_text": kind + " " + c.SavingsNo}
	_, err = p.db.Collection("account_transaction_feed").UpdateOne(ctx, bson.M{"account_id": accountID, "transaction_id": m.TransactionID}, bson.M{"$set": feed}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		return err
	}
	row := bson.M{"savings_no": c.SavingsNo, "customer_id": c.CustomerID, "package_id": c.PackageID, "source_account_id": c.SourceAccountID, "principal": c.Principal, "term_months": c.TermMonths, "annual_rate_bps": c.AnnualRateBps, "opened_at": c.OpenedAt, "maturity_at": c.MaturityAt, "status": c.Status, "opening_quote_id": m.OpeningQuoteID, "opening_idempotency_key": m.OpeningKey, "updated_at": record.At, "financial_revision": sequence}
	if c.Settlement != nil {
		row["settlement"] = bson.M{"transaction_id": c.Settlement.TransactionID, "interest_amount": c.Settlement.InterestAmount, "total_amount": c.Settlement.TotalAmount, "settled_at": c.Settlement.SettledAt}
	}
	_, err = p.db.Collection("savings_contracts").UpdateOne(ctx, bson.M{"_id": c.ID}, bson.M{"$set": row}, options.UpdateOne().SetUpsert(true))
	return err
}
