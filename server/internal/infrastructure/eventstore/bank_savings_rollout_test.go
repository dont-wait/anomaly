package eventstore

import (
	"context"
	"errors"
	"testing"
	"time"

	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
)

func TestSavingsEventWritesRemainDisabledUntilExplicitlyEnabled(t *testing.T) {
	t.Setenv("SAVINGS_EVENT_WRITE_ENABLED", "")
	r := &BankRepository{}
	if _, err := r.OpenSavings(context.Background(), journal.SavingsOpenInput{}); !errors.Is(err, ErrSavingsWritesDisabled) {
		t.Fatalf("opening must fail closed: %v", err)
	}
	if _, err := r.SettleSavings(context.Background(), "", "", "", "", time.Time{}); !errors.Is(err, ErrSavingsWritesDisabled) {
		t.Fatalf("settlement must fail closed: %v", err)
	}
}
