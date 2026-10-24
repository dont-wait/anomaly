package eventstore

import (
	"errors"
	"testing"

	"github.com/dont-wait/anomaly/internal/domain/savings"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
)

func TestConfirmSavingsSettlementUsesPersistedRequest(t *testing.T) {
	const (
		contractID = "contract-1"
		winningKey = "actor:key-1"
		losingKey  = "actor:key-2"
		quoteID    = "quote-1"
	)
	committed := journal.SavingsMovement{
		Contract:          savings.Contract{ID: contractID, Status: savings.ContractSettled, Settlement: &savings.Settlement{TransactionID: "tx-1"}},
		WithdrawalKey:     winningKey,
		WithdrawalQuoteID: quoteID,
		TransactionID:     "tx-1",
	}
	state := &bankState{Savings: map[string]journal.SavingsMovement{contractID: committed}}

	got, err := confirmSavingsSettlement(state, contractID, quoteID, winningKey)
	if err != nil || got.TransactionID != committed.TransactionID {
		t.Fatalf("persisted winner must succeed: movement=%+v err=%v", got, err)
	}
	if _, err := confirmSavingsSettlement(state, contractID, quoteID, losingKey); !errors.Is(err, savings.ErrAlreadySettled) {
		t.Fatalf("another key must not claim the persisted settlement: %v", err)
	}
	if _, err := confirmSavingsSettlement(state, contractID, "another-quote", winningKey); !errors.Is(err, savings.ErrAlreadySettled) {
		t.Fatalf("another quote must not claim the persisted settlement: %v", err)
	}
}

func TestConfirmSavingsSettlementRequiresDurableReceipt(t *testing.T) {
	state := &bankState{Savings: map[string]journal.SavingsMovement{
		"pending": {Contract: savings.Contract{ID: "pending", Status: savings.ContractActive}},
	}}
	if _, err := confirmSavingsSettlement(state, "pending", "quote", "actor:key"); err == nil {
		t.Fatal("must not confirm an uncommitted settlement")
	}
	if _, err := confirmSavingsSettlement(state, "missing", "quote", "actor:key"); err == nil {
		t.Fatal("must not confirm a missing settlement")
	}
}
