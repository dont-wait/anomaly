package eventstore

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/dont-wait/anomaly/internal/domain/savings"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
)

func testSavingsOpeningConfirmationFixture() (journal.SavingsOpenInput, *bankState) {
	in := journal.SavingsOpenInput{
		ActorID: "actor-1", IdempotencyKey: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		QuoteID: "quote-1", SourceAccountID: "account-1", CustomerID: "customer-1",
		PackageID: "savings_standard", Principal: 1_000_000, TermMonths: 3, AnnualRateBps: 450,
	}
	key := in.ActorID + ":" + in.IdempotencyKey
	m := journal.SavingsMovement{
		ActorID: in.ActorID, OpeningKey: key, OpeningQuoteID: in.QuoteID,
		Contract: savings.Contract{
			ID: "contract-1", CustomerID: in.CustomerID, SourceAccountID: in.SourceAccountID,
			PackageID: in.PackageID, Principal: in.Principal, TermMonths: in.TermMonths,
			AnnualRateBps: in.AnnualRateBps,
		},
	}
	return in, &bankState{
		SavingsKeys: map[string]string{key: m.Contract.ID},
		Savings:     map[string]journal.SavingsMovement{m.Contract.ID: m},
	}
}

func TestConfirmSavingsOpeningReturnsOnlyPersistedPayload(t *testing.T) {
	in, state := testSavingsOpeningConfirmationFixture()
	got, err := confirmSavingsOpening(state, in)
	if err != nil || got.Contract.ID != "contract-1" {
		t.Fatalf("identical retry should return stored contract: movement=%+v err=%v", got, err)
	}
	cases := []struct {
		name string
		edit func(*journal.SavingsOpenInput)
	}{
		{"quote", func(x *journal.SavingsOpenInput) { x.QuoteID = "other-quote" }},
		{"principal", func(x *journal.SavingsOpenInput) { x.Principal++ }},
		{"term", func(x *journal.SavingsOpenInput) { x.TermMonths = 6 }},
		{"source", func(x *journal.SavingsOpenInput) { x.SourceAccountID = "other-account" }},
		{"customer", func(x *journal.SavingsOpenInput) { x.CustomerID = "other-customer" }},
		{"package", func(x *journal.SavingsOpenInput) { x.PackageID = "other-package" }},
		{"rate", func(x *journal.SavingsOpenInput) { x.AnnualRateBps++ }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conflicting := in
			tc.edit(&conflicting)
			if _, err := confirmSavingsOpening(state, conflicting); !errors.Is(err, ErrSavingsIdempotencyConflict) {
				t.Fatalf("same key/different %s must conflict, got %v", tc.name, err)
			}
		})
	}
	delete(state.Savings, "contract-1")
	if _, err := confirmSavingsOpening(state, in); err == nil || errors.Is(err, ErrSavingsIdempotencyConflict) {
		t.Fatalf("missing persisted opening cannot succeed or be called a conflict: %v", err)
	}
}

func TestConfirmSavingsOpeningRecoveryAndUnknownOutcome(t *testing.T) {
	in, state := testSavingsOpeningConfirmationFixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // The request may have disappeared after EventStoreDB accepted its write.
	reads := 0
	movement, err := confirmSavingsOpeningWithRecovery(ctx, func(recovery context.Context) (*bankState, error) {
		if recovery.Err() != nil {
			t.Fatalf("confirmation must use a fresh bounded context: %v", recovery.Err())
		}
		reads++
		if reads == 1 {
			return nil, fmt.Errorf("transient stream read error")
		}
		return state, nil
	}, in)
	if err != nil || reads != 2 || movement.Contract.ID != "contract-1" {
		t.Fatalf("retry must return persisted opening: movement=%+v err=%v reads=%d", movement, err, reads)
	}
	conflict := in
	conflict.QuoteID = "different"
	if _, err := confirmSavingsOpeningWithRecovery(context.Background(), func(context.Context) (*bankState, error) {
		return state, nil
	}, conflict); !errors.Is(err, ErrSavingsIdempotencyConflict) {
		t.Fatalf("losing request cannot receive success: %v", err)
	}
	reads = 0
	if _, err := confirmSavingsOpeningWithRecovery(context.Background(), func(context.Context) (*bankState, error) {
		reads++
		return nil, fmt.Errorf("persistent EventStoreDB outage")
	}, in); !errors.Is(err, ErrSavingsOpeningUnconfirmed) || reads != 4 {
		t.Fatalf("read outage must remain unknown, err=%v reads=%d", err, reads)
	}
}
