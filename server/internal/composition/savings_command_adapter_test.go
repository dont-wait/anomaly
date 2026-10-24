package composition

import (
	"errors"
	"testing"

	savingscommands "github.com/dont-wait/anomaly/internal/application/savings/commands"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
)

func TestSavingsRepositoryErrorsTranslatedAtCompositionBoundary(t *testing.T) {
	cases := []struct {
		from error
		to   error
	}{
		{eventstore.ErrSavingsNotFound, savingscommands.ErrSavingsNotFound},
		{eventstore.ErrSavingsQuoteUsed, savingscommands.ErrIdempotencyConflict},
		{eventstore.ErrSavingsIdempotencyConflict, savingscommands.ErrIdempotencyConflict},
		{eventstore.ErrSavingsSourceUnavailable, savingscommands.ErrSavingsSourceUnavailable},
		{eventstore.ErrSavingsInsufficientFunds, savingscommands.ErrSavingsInsufficientFunds},
		{eventstore.ErrSavingsQuoteExpired, savingscommands.ErrWithdrawalQuoteExpired},
		{eventstore.ErrSavingsWritesDisabled, savingscommands.ErrSavingsWritesDisabled},
	}
	for _, c := range cases {
		if got := savingsCommandError(c.from); !errors.Is(got, c.to) {
			t.Errorf("%v mapped to %v, wanted %v", c.from, got, c.to)
		}
	}
}
