package transaction

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/savings"
)

func TestSavingsJournalAtomicOpenAndSettlement(t *testing.T) {
	start := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	c, err := savings.NewContract(savings.OpenContractParams{ID: EventID("case", "contract"), SavingsNo: "SV123", CustomerID: "customer", PackageID: "savings_standard", SourceAccountID: "account", Principal: 1_000_000, TermMonths: 3, AnnualRateBps: 450, OpenedAt: start})
	if err != nil {
		t.Fatal(err)
	}
	m := SavingsMovement{Contract: *c, ActorID: "actor", OpeningQuoteID: "quote", OpeningKey: "actor:key", TransactionID: EventID(c.ID, "deposit")}
	before, after := int64(2_000_000), int64(1_000_000)
	r := Record{ID: EventID(c.ID, SavingsOpened), Type: SavingsOpened, At: start, AccountID: c.SourceAccountID, BalanceBefore: &before, BalanceAfter: &after, Savings: &m}
	raw, _ := json.Marshal(r)
	if _, err := DecodeRecord(raw, r.ID, r.Type); err != nil {
		t.Fatalf("valid open: %v", err)
	}
	after++ // wrong balance must reject
	r.BalanceAfter = &after
	raw, _ = json.Marshal(r)
	if _, err := DecodeRecord(raw, r.ID, r.Type); err == nil {
		t.Fatal("accepted non-debiting savings opening")
	}
	now := c.MaturityAt.Add(time.Hour)
	c.Revision = 10
	intent, err := c.PrepareWithdrawal(now)
	if err != nil {
		t.Fatal(err)
	}
	tx := EventID(c.ID, "withdrawal")
	if err := c.ConfirmSettled(intent, tx, 11, now); err != nil {
		t.Fatal(err)
	}
	m.Contract = *c
	m.WithdrawalKey = "actor:key2"
	m.WithdrawalQuoteID = "withdraw-quote"
	m.TransactionID = tx
	before, after = 100, intent.TotalAmount+100
	r = Record{ID: EventID(c.ID, SavingsSettled), Type: SavingsSettled, At: now, AccountID: c.SourceAccountID, BalanceBefore: &before, BalanceAfter: &after, Savings: &m}
	raw, _ = json.Marshal(r)
	if _, err := DecodeRecord(raw, r.ID, r.Type); err != nil {
		t.Fatalf("valid settlement: %v", err)
	}
	after++
	raw, _ = json.Marshal(r)
	if _, err := DecodeRecord(raw, r.ID, r.Type); err == nil {
		t.Fatal("accepted inflated savings credit")
	}
}
