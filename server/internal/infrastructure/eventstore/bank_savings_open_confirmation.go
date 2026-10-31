package eventstore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
)

// savingsOpeningMatchesInput rejects reuse of the same idempotency key for
// another opening, even when the two requests share the same EventID.
func savingsOpeningMatchesInput(m journal.SavingsMovement, in journal.SavingsOpenInput) bool {
	c := m.Contract
	key := in.ActorID + ":" + strings.ToLower(in.IdempotencyKey)
	return c.ID != "" && m.OpeningKey == key && m.ActorID == in.ActorID &&
		m.OpeningQuoteID == in.QuoteID && c.CustomerID == in.CustomerID &&
		c.SourceAccountID == in.SourceAccountID && c.PackageID == in.PackageID &&
		c.Principal == in.Principal && c.TermMonths == in.TermMonths &&
		c.AnnualRateBps == in.AnnualRateBps
}

func confirmSavingsOpening(s *bankState, in journal.SavingsOpenInput) (journal.SavingsMovement, error) {
	if s == nil {
		return journal.SavingsMovement{}, errors.New("missing authoritative banking state")
	}
	key := in.ActorID + ":" + strings.ToLower(in.IdempotencyKey)
	id := s.SavingsKeys[key]
	m, ok := s.Savings[id]
	if id == "" || !ok {
		return journal.SavingsMovement{}, errors.New("savings opening not yet confirmed in banking stream")
	}
	if !savingsOpeningMatchesInput(m, in) {
		return journal.SavingsMovement{}, ErrSavingsIdempotencyConflict
	}
	return m, nil
}

// A write acknowledgement alone is insufficient: concurrent appends with the
// same deterministic EventID can return success for different local payloads.
// Read the leader-backed canonical state and return only the actual winner.
func confirmSavingsOpeningWithRecovery(ctx context.Context, read func(context.Context) (*bankState, error), in journal.SavingsOpenInput) (journal.SavingsMovement, error) {
	const maxReads = 4
	recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	var lastErr error
	for attempt := 0; attempt < maxReads; attempt++ {
		state, err := read(recoveryCtx)
		if err == nil {
			var movement journal.SavingsMovement
			movement, err = confirmSavingsOpening(state, in)
			if err == nil || errors.Is(err, ErrSavingsIdempotencyConflict) {
				return movement, err
			}
		}
		lastErr = err
		if attempt == maxReads-1 {
			break
		}
		timer := time.NewTimer(time.Duration(50<<attempt) * time.Millisecond)
		select {
		case <-recoveryCtx.Done():
			timer.Stop()
			return journal.SavingsMovement{}, fmt.Errorf("%w: %v", ErrSavingsOpeningUnconfirmed, lastErr)
		case <-timer.C:
		}
	}
	return journal.SavingsMovement{}, fmt.Errorf("%w: %v", ErrSavingsOpeningUnconfirmed, lastErr)
}
