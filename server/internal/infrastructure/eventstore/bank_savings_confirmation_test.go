package eventstore

import (
	"context"
	"errors"
	"fmt"
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

func TestSavingsSettlementConfirmationRetriesAndPreservesUnknown(t *testing.T) {
	const contractID = "contract-1"
	const quoteID = "quote-1"
	const winningKey = "actor:key-1"
	committed := journal.SavingsMovement{
		Contract:      savings.Contract{ID: contractID, Status: savings.ContractSettled, Settlement: &savings.Settlement{TransactionID: "tx"}},
		WithdrawalKey: winningKey, WithdrawalQuoteID: quoteID, TransactionID: "tx",
	}
	state := &bankState{Savings: map[string]journal.SavingsMovement{contractID: committed}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // HTTP request is gone after an accepted append.
	reads := 0
	m, err := confirmSavingsSettlementWithRecovery(ctx, func(recovery context.Context) (*bankState, error) {
		if err := recovery.Err(); err != nil {
			t.Fatalf("recovery context must survive cancelled request: %v", err)
		}
		reads++
		if reads == 1 {
			return nil, fmt.Errorf("temporary EventStoreDB read failure")
		}
		return state, nil
	}, contractID, quoteID, winningKey)
	if err != nil || m.TransactionID != "tx" || reads != 2 {
		t.Fatalf("confirmed after retry: movement=%+v err=%v reads=%d", m, err, reads)
	}
	if _, err := confirmSavingsSettlementWithRecovery(context.Background(), func(context.Context) (*bankState, error) {
		return state, nil
	}, contractID, quoteID, "actor:loser"); !errors.Is(err, savings.ErrAlreadySettled) {
		t.Fatalf("wrong request claimed committed withdrawal: %v", err)
	}
	reads = 0
	if _, err := confirmSavingsSettlementWithRecovery(context.Background(), func(context.Context) (*bankState, error) {
		reads++
		return nil, fmt.Errorf("persistent EventStoreDB outage")
	}, contractID, quoteID, winningKey); !errors.Is(err, ErrSavingsSettlementUnconfirmed) || reads != 4 {
		t.Fatalf("must report unknown instead of failed settlement: err=%v reads=%d", err, reads)
	}
}
