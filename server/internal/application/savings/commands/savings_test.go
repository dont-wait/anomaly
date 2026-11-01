package commands

import (
	"context"
	"errors"
	"testing"

	savingsqueries "github.com/dont-wait/anomaly/internal/application/savings/queries"
	"github.com/dont-wait/anomaly/internal/domain/savings"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/google/uuid"
)

func TestSavingsCommandsRequireConfirmationAndUUIDv4(t *testing.T) {
	valid := uuid.NewString()
	if err := validateKey(valid); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"", "not-a-uuid", uuid.NewSHA1(uuid.NameSpaceOID, []byte("v5")).String()} {
		if err := validateKey(key); !errors.Is(err, ErrInvalidKey) {
			t.Fatalf("invalid key %q accepted: %v", key, err)
		}
	}
	s := &Service{}
	// Invalid keys must be rejected by the command entry points themselves,
	// even if confirmation is true (not only by the validateKey helper).
	for _, invalid := range []string{"", "not-a-uuid", uuid.NewSHA1(uuid.NameSpaceOID, []byte("v5")).String()} {
		if _, err := s.Open(context.Background(), "actor", invalid, OpenInput{Confirm: true}); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("Open accepted invalid key %q: %v", invalid, err)
		}
		if _, err := s.Withdraw(context.Background(), "actor", "contract", invalid, WithdrawInput{Confirm: true}); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("Withdraw accepted invalid key %q: %v", invalid, err)
		}
	}
	if _, err := s.Open(context.Background(), "actor", valid, OpenInput{Confirm: false}); !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("opening without confirmation: %v", err)
	}
	if _, err := s.Withdraw(context.Background(), "actor", "contract", valid, WithdrawInput{Confirm: false}); !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("withdrawal without confirmation: %v", err)
	}
	if _, err := s.WithdrawalPreview(context.Background(), "actor", "contract", "partial"); !errors.Is(err, ErrUnsupportedWithdrawalType) {
		t.Fatalf("partial withdrawal: %v", err)
	}
}

type consentTestBank struct{ Bank }

func (consentTestBank) FindSavingsOpening(context.Context, string, string) (journal.SavingsMovement, bool, error) {
	return journal.SavingsMovement{}, false, nil
}

type consentTestTerms struct{}

func (consentTestTerms) Current(context.Context) (savings.TermsSnapshot, error) {
	return savings.TermsSnapshot{
		Version: "terms-v2", Title: "Terms", Introduction: "Savings",
		Sections: []savings.TermsSection{{Title: "Period", Body: "Maturity"}},
	}, nil
}

func TestOpenRejectsStaleAndMissingTermsBeforeFundsMove(t *testing.T) {
	svc := &Service{Bank: consentTestBank{}, Quotes: &savingsqueries.Service{}, Terms: consentTestTerms{}}
	for _, version := range []string{"", "terms-v1"} {
		_, err := svc.Open(context.Background(), "actor", "9c9a9ad3-bab1-4385-8ce9-b9c694ecdf4d", OpenInput{
			QuoteID: "q", SourceAccountID: "a", Confirm: true, AcceptedTermsVersion: version,
		})
		if !errors.Is(err, ErrTermsChanged) {
			t.Fatalf("version %q: got %v", version, err)
		}
	}
}
