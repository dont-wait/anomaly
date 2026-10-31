package commands

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/domain/savings"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
)

type settlementRaceBank struct {
	opening journal.SavingsMovement
	calls   int
}

func (b *settlementRaceBank) FindSavings(_ context.Context, _, _ string) (journal.SavingsMovement, error) {
	b.calls++
	m := b.opening
	if b.calls > 1 {
		m.Contract.Status = savings.ContractSettled
	}
	return m, nil
}

func (b *settlementRaceBank) FindByID(_ context.Context, id string) (*account.UserAccount, error) {
	return &account.UserAccount{
		Id: id, Status: account.AccountStatusActive,
		Type: account.AccountTypePayment, Currency: account.CurrencyVND,
	}, nil
}

func (*settlementRaceBank) OpenSavings(context.Context, journal.SavingsOpenInput) (journal.SavingsMovement, error) {
	return journal.SavingsMovement{}, errors.New("unexpected open call")
}

func (*settlementRaceBank) FindSavingsOpening(context.Context, string, string) (journal.SavingsMovement, bool, error) {
	return journal.SavingsMovement{}, false, errors.New("unexpected lookup")
}

func (*settlementRaceBank) SettleSavings(context.Context, string, string, string, string, time.Time) (journal.SavingsMovement, error) {
	return journal.SavingsMovement{}, errors.New("unexpected settle call")
}

type raceQuoteStore struct{ saved WithdrawalQuote }

func (q *raceQuoteStore) SaveWithdrawal(_ context.Context, item WithdrawalQuote) error {
	q.saved = item
	return nil
}

func (q *raceQuoteStore) GetWithdrawal(_ context.Context, id string) (WithdrawalQuote, bool, error) {
	return q.saved, q.saved.ID == id, nil
}

func TestWithdrawalPreviewRejectsSettlementDuringQuoteSave(t *testing.T) {
	openedAt := time.Date(2025, 1, 31, 7, 0, 0, 0, time.UTC)
	contract, err := savings.NewContract(savings.OpenContractParams{
		ID: "savings-id", SavingsNo: "SV001", CustomerID: "owner", PackageID: "savings_standard",
		SourceAccountID: "source", Principal: 1_000_000, TermMonths: 3,
		AnnualRateBps: 450, OpenedAt: openedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	bank := &settlementRaceBank{opening: journal.SavingsMovement{Contract: *contract, ActorID: "actor"}}
	store := &raceQuoteStore{}
	svc := &Service{
		Bank: bank, WithdrawalQuotes: store,
		Now: func() time.Time { return contract.MaturityAt.Add(time.Second) },
	}
	_, err = svc.WithdrawalPreview(context.Background(), "actor", contract.ID, "full")
	if !errors.Is(err, savings.ErrAlreadySettled) || bank.calls != 2 {
		t.Fatalf("preview returned stale quote during settlement, calls=%d err=%v", bank.calls, err)
	}
}

func TestOpeningIdempotentResponseUsesStoredContractStatus(t *testing.T) {
	m := journal.SavingsMovement{Contract: savings.Contract{Status: savings.ContractSettled}}
	if got := opened(m).Status; got != savings.ContractSettled {
		t.Fatalf("replay status=%s want settled", got)
	}
}

// settledWithdrawalBank returns the persisted EventStoreDB settlement directly.
// Retries must be decided before trying to fetch an expired withdrawal quote.
type settledWithdrawalBank struct {
	settlementRaceBank
	persisted journal.SavingsMovement
}

func (b *settledWithdrawalBank) FindSavings(context.Context, string, string) (journal.SavingsMovement, error) {
	return b.persisted, nil
}

func TestWithdrawIdempotencyUsesPersistedQuote(t *testing.T) {
	const actor = "actor"
	const contractID = "contract"
	const quoteID = "persisted-quote"
	key := "5d94122b-135b-4e6d-bf68-2713b8ca8d58"
	m := journal.SavingsMovement{
		Contract: savings.Contract{
			ID: contractID, Status: savings.ContractSettled,
			Principal: 1_000_000, SourceAccountID: "account",
			Settlement: &savings.Settlement{TransactionID: "tx", InterestAmount: 12_000, TotalAmount: 1_012_000},
		},
		WithdrawalKey: actor + ":" + key, WithdrawalQuoteID: quoteID,
		TransactionID: "tx",
	}
	bank := &settledWithdrawalBank{persisted: m}
	svc := &Service{Bank: bank, WithdrawalQuotes: &raceQuoteStore{}}

	// The original quote isn't in Redis any more; the same request still
	// receives its persisted receipt, without another SettleSavings call.
	got, err := svc.Withdraw(context.Background(), actor, contractID, key, WithdrawInput{QuoteID: quoteID, Confirm: true})
	if err != nil || got.TransactionID != "tx" || got.TotalPayout != 1_012_000 {
		t.Fatalf("replay settled request: receipt=%+v err=%v", got, err)
	}
	if _, err := svc.Withdraw(context.Background(), actor, contractID, key, WithdrawInput{QuoteID: "different-quote", Confirm: true}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("same idempotency key with changed quote: %v", err)
	}
	if _, err := svc.Withdraw(context.Background(), actor, contractID, "c65e7c73-f925-49b8-8d55-2ae1477d84f9", WithdrawInput{QuoteID: quoteID, Confirm: true}); !errors.Is(err, savings.ErrAlreadySettled) {
		t.Fatalf("different idempotency key must remain already settled: %v", err)
	}
}
