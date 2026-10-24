package commands

import (
	"context"
	"errors"
	"testing"

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
