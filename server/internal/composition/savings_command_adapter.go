package composition

import (
	"context"
	"errors"
	"time"

	savingscommands "github.com/dont-wait/anomaly/internal/application/savings/commands"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
)

// savingsBankCommandAdapter translates adapter-specific failures into the
// Savings Application contract; HTTP must not import infrastructure errors.
type savingsBankCommandAdapter struct{ *eventstore.BankRepository }

func savingsCommandError(err error) error {
	switch {
	case errors.Is(err, eventstore.ErrSavingsNotFound):
		return savingscommands.ErrSavingsNotFound
	case errors.Is(err, eventstore.ErrSavingsIdempotencyConflict), errors.Is(err, eventstore.ErrSavingsQuoteUsed):
		return savingscommands.ErrIdempotencyConflict
	case errors.Is(err, eventstore.ErrSavingsSourceUnavailable):
		return savingscommands.ErrSavingsSourceUnavailable
	case errors.Is(err, eventstore.ErrSavingsInsufficientFunds):
		return savingscommands.ErrSavingsInsufficientFunds
	case errors.Is(err, eventstore.ErrSavingsQuoteExpired):
		return savingscommands.ErrWithdrawalQuoteExpired
	case errors.Is(err, eventstore.ErrSavingsWritesDisabled):
		return savingscommands.ErrSavingsWritesDisabled
	case errors.Is(err, eventstore.ErrSavingsSettlementUnconfirmed):
		return savingscommands.ErrSavingsSettlementUnconfirmed
	default:
		return err
	}
}

func (a savingsBankCommandAdapter) OpenSavings(ctx context.Context, in journal.SavingsOpenInput) (journal.SavingsMovement, error) {
	m, err := a.BankRepository.OpenSavings(ctx, in)
	return m, savingsCommandError(err)
}

func (a savingsBankCommandAdapter) FindSavingsOpening(ctx context.Context, actor, key string) (journal.SavingsMovement, bool, error) {
	m, found, err := a.BankRepository.FindSavingsOpening(ctx, actor, key)
	return m, found, savingsCommandError(err)
}

func (a savingsBankCommandAdapter) FindSavings(ctx context.Context, actor, id string) (journal.SavingsMovement, error) {
	m, err := a.BankRepository.FindSavings(ctx, actor, id)
	return m, savingsCommandError(err)
}

func (a savingsBankCommandAdapter) SettleSavings(ctx context.Context, actor, id, quoteID, key string, expiry time.Time) (journal.SavingsMovement, error) {
	m, err := a.BankRepository.SettleSavings(ctx, actor, id, quoteID, key, expiry)
	return m, savingsCommandError(err)
}
