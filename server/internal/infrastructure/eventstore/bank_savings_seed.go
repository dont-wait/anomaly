package eventstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/domain/savings"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
)

// SeedSavingsInput describes a development-only savings contract opened in the
// past, so a demo can exercise maturity withdrawal without waiting a full term.
type SeedSavingsInput struct {
	AccountID      string
	PackageID      string
	PackageVersion int
	QuoteID        string
	IdempotencyKey string
	Principal      int64
	TermMonths     int
	AnnualRateBps  int64
	OpenedAt       time.Time
}

// SeedBackdatedSavings appends a SavingsOpened event whose contract opened at
// in.OpenedAt. It bypasses quote expiry and the opening clock, which only the
// seed CLI may do. Funds, ownership and uniqueness are still validated against
// the canonical banking stream, and a repeated key returns the stored contract.
func (r SeedRepository) SeedBackdatedSavings(ctx context.Context, in SeedSavingsInput) (journal.SavingsMovement, bool, error) {
	key := in.AccountID + ":" + strings.ToLower(in.IdempotencyKey)
	created := false
	result, err := r.mutate(ctx, func(s *bankState) (*journal.Record, any, error) {
		if id := s.SavingsKeys[key]; id != "" {
			return nil, s.Savings[id], nil
		}
		if s.SavingsQuotes[in.QuoteID] != "" {
			return nil, nil, ErrSavingsQuoteUsed
		}
		owner := s.Accounts[in.AccountID]
		if owner == nil || owner.Status != account.AccountStatusActive ||
			owner.Type != account.AccountTypePayment || owner.Currency != account.CurrencyVND {
			return nil, nil, ErrSavingsSourceUnavailable
		}
		id := journal.EventID(key, "savings-contract")
		no := "SV" + strings.ToUpper(strings.ReplaceAll(id, "-", ""))[:18]
		c, err := savings.NewContract(savings.OpenContractParams{
			ID: id, SavingsNo: no, CustomerID: owner.CustomerId, PackageID: in.PackageID,
			PackageVersion: in.PackageVersion, SourceAccountID: owner.Id, Principal: in.Principal,
			TermMonths: in.TermMonths, AnnualRateBps: in.AnnualRateBps, OpenedAt: in.OpenedAt,
		})
		if err != nil {
			return nil, nil, err
		}
		accountKey := financialID(owner.Id).Hex()
		before := s.Balances[accountKey]
		if before < in.Principal {
			return nil, nil, fmt.Errorf("%w: balance %d < principal %d", ErrSavingsInsufficientFunds, before, in.Principal)
		}
		after := before - in.Principal
		m := journal.SavingsMovement{
			Contract: *c, ActorID: owner.Id, OpeningQuoteID: in.QuoteID, OpeningKey: key,
			TransactionID: journal.EventID(id, "deposit"),
		}
		created = true
		return &journal.Record{
			ID: journal.EventID(id, journal.SavingsOpened), Type: journal.SavingsOpened, At: in.OpenedAt,
			AccountID: owner.Id, BalanceBefore: &before, BalanceAfter: &after, Savings: &m,
		}, m, nil
	})
	if err != nil {
		return journal.SavingsMovement{}, false, err
	}
	m, ok := result.(journal.SavingsMovement)
	if !ok {
		return journal.SavingsMovement{}, false, fmt.Errorf("unexpected seeded savings result")
	}
	return m, created, nil
}
