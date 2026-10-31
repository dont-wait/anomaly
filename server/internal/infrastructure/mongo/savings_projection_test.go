package mongo

import (
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/savings"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
)

func TestSavingsContractRowHasConsistentSettlement(t *testing.T) {
	now := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	c := savings.Contract{ID: "savings-1", Status: savings.ContractActive}
	m := journal.SavingsMovement{OpeningQuoteID: "q", OpeningKey: "k"}
	row, err := savingsContractRow(c, m, 5, now)
	if err != nil || row["settlement"] != nil || row["financial_revision"] != int64(5) {
		t.Fatalf("invalid opening projection %v / %v", row, err)
	}
	c.Status = savings.ContractSettled
	if _, err := savingsContractRow(c, m, 6, now); err == nil {
		t.Fatal("accepted settled contract without receipt")
	}
	c.Settlement = &savings.Settlement{TransactionID: "tx", TotalAmount: 10, SettledAt: now}
	row, err = savingsContractRow(c, m, 6, now)
	if err != nil || row["settlement"] == nil {
		t.Fatalf("missing settlement: %v / %v", row, err)
	}
	c.Status = savings.ContractActive
	if _, err := savingsContractRow(c, m, 7, now); err == nil {
		t.Fatal("accepted active contract with prior settlement receipt")
	}
}
