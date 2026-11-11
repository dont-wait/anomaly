package eventstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/google/uuid"
)

// HistoricalTransferInput describes a completed transfer that belongs to a
// development/test fixture. Historical events are retained in the canonical
// banking stream and projections, but do not change the account's current
// canonical balance. This lets a fixture describe a past balance trajectory
// while leaving the account funded for the manual probe.
type HistoricalTransferInput struct {
	ID                       string
	IdempotencyKey           string
	SourceAccountID          string
	DestinationAccountID     string
	Amount                   int64
	Fee                      int64
	Currency                 string
	Channel                  string
	DeviceID                 string
	Platform                 string
	Note                     string
	OccurredAt               time.Time
	CompletedAt              time.Time
	SourceBalanceBefore      int64
	SourceBalanceAfter       int64
	DestinationBalanceBefore int64
	DestinationBalanceAfter  int64
}

// SeedHistoricalTransfer appends an idempotent TransferCreated ->
// TransferCompleted pair for a development/test fixture. It intentionally
// uses the same journal format as live transfers instead of writing MongoDB
// documents directly.
func (r *BankRepository) SeedHistoricalTransfer(ctx context.Context, in HistoricalTransferInput) error {
	if err := validateHistoricalTransferInput(in); err != nil {
		return err
	}

	if err := r.seedHistoricalCreated(ctx, in); err != nil {
		return err
	}
	return r.seedHistoricalCompleted(ctx, in)
}

func validateHistoricalTransferInput(in HistoricalTransferInput) error {
	if _, err := uuid.Parse(in.ID); err != nil {
		return fmt.Errorf("historical transfer id: %w", err)
	}
	if strings.TrimSpace(in.IdempotencyKey) == "" {
		return fmt.Errorf("historical transfer %s: idempotency key is required", in.ID)
	}
	if in.SourceAccountID == "" || in.DestinationAccountID == "" || in.SourceAccountID == in.DestinationAccountID {
		return fmt.Errorf("historical transfer %s: invalid account pair", in.ID)
	}
	if in.Amount < 1000 || in.Fee < 0 || in.Currency != "VND" {
		return fmt.Errorf("historical transfer %s: invalid money fields", in.ID)
	}
	if in.Channel != "web" && in.Channel != "mobile" && in.Channel != "desktop" {
		return fmt.Errorf("historical transfer %s: invalid channel %q", in.ID, in.Channel)
	}
	if in.OccurredAt.IsZero() || in.CompletedAt.IsZero() || !in.CompletedAt.After(in.OccurredAt) {
		return fmt.Errorf("historical transfer %s: invalid timestamps", in.ID)
	}
	if in.SourceBalanceBefore < in.Amount+in.Fee || in.SourceBalanceAfter != in.SourceBalanceBefore-in.Amount-in.Fee {
		return fmt.Errorf("historical transfer %s: invalid source balance snapshots", in.ID)
	}
	if in.DestinationBalanceBefore < 0 || in.DestinationBalanceAfter != in.DestinationBalanceBefore+in.Amount {
		return fmt.Errorf("historical transfer %s: invalid destination balance snapshots", in.ID)
	}
	return nil
}

func (r *BankRepository) seedHistoricalCreated(ctx context.Context, in HistoricalTransferInput) error {
	_, err := r.mutate(ctx, func(s *bankState) (*journal.Record, any, error) {
		source := s.Accounts[in.SourceAccountID]
		destination := s.Accounts[in.DestinationAccountID]
		if source == nil || destination == nil {
			return nil, nil, fmt.Errorf("historical transfer %s: account not found", in.ID)
		}

		if existingID := s.Keys[strings.ToLower(in.IdempotencyKey)]; existingID != "" {
			existing, ok := s.Transfers[existingID]
			if !ok || !sameHistoricalTransfer(existing.Tx, in) {
				return nil, nil, fmt.Errorf("historical transfer %s: idempotency conflict", in.ID)
			}
			return nil, nil, nil
		}
		if existing, ok := s.Transfers[in.ID]; ok && !sameHistoricalTransfer(existing.Tx, in) {
			return nil, nil, fmt.Errorf("historical transfer %s: transaction conflict", in.ID)
		}

		tx, err := historicalTransferDocument(source, destination, in, "awaiting_otp")
		if err != nil {
			return nil, nil, err
		}
		e := eventFor(tx, journal.Created, 1, in.OccurredAt)
		return historicalTransferRecord(tx, e)
	})
	return err
}

func (r *BankRepository) seedHistoricalCompleted(ctx context.Context, in HistoricalTransferInput) error {
	_, err := r.mutate(ctx, func(s *bankState) (*journal.Record, any, error) {
		state, ok := s.Transfers[in.ID]
		if !ok {
			return nil, nil, fmt.Errorf("historical transfer %s: creation event missing", in.ID)
		}
		if state.Tx.Status == "success" {
			if sameHistoricalTransfer(state.Tx, in) {
				return nil, nil, nil
			}
			return nil, nil, fmt.Errorf("historical transfer %s: completed transaction conflict", in.ID)
		}
		if state.Tx.Status != "awaiting_otp" || state.Event.Sequence != 1 {
			return nil, nil, fmt.Errorf("historical transfer %s: invalid creation state", in.ID)
		}

		source := s.Accounts[in.SourceAccountID]
		destination := s.Accounts[in.DestinationAccountID]
		tx, err := historicalTransferDocument(source, destination, in, "success")
		if err != nil {
			return nil, nil, err
		}
		tx.PostedAt = in.CompletedAt
		tx.CompletedAt = &in.CompletedAt
		tx.OTPVerifiedAt = &in.CompletedAt
		tx.Authentication.Method = "email_otp"
		tx.Authentication.Verified = true
		e := eventFor(tx, journal.Completed, 2, in.CompletedAt)
		e.Payload.SourceBalanceBefore = int64Ptr(in.SourceBalanceBefore)
		e.Payload.SourceBalanceAfter = int64Ptr(in.SourceBalanceAfter)
		e.Payload.DestinationBalanceBefore = int64Ptr(in.DestinationBalanceBefore)
		e.Payload.DestinationBalanceAfter = int64Ptr(in.DestinationBalanceAfter)
		return historicalTransferRecord(tx, e)
	})
	return err
}

func historicalTransferDocument(source, destination *accountdomain.UserAccount, in HistoricalTransferInput, status string) (mongorepo.TransferDocument, error) {
	from, err := transferAccount(source)
	if err != nil {
		return mongorepo.TransferDocument{}, err
	}
	to, err := transferAccount(destination)
	if err != nil {
		return mongorepo.TransferDocument{}, err
	}
	tx := mongorepo.TransferDocument{
		ID: in.ID, Reference: historicalTransferReference(in.ID), Type: "transfer",
		Amount: in.Amount, Fee: in.Fee, Currency: in.Currency, Channel: in.Channel,
		DeviceID: in.DeviceID, Platform: in.Platform, Status: status, Note: in.Note,
		IdempotencyKey: strings.ToLower(in.IdempotencyKey), CreatedAt: in.OccurredAt,
		OTPExpiresAt: in.OccurredAt.Add(5 * time.Minute), OTPResendAt: in.OccurredAt.Add(30 * time.Second),
	}
	tx.Source.AccountID, tx.Source.AccountNo, tx.Source.Name = from.ID, from.AccountNo, from.Name
	tx.Destination.Type, tx.Destination.AccountID, tx.Destination.AccountNo = "internal", to.ID, to.AccountNo
	tx.Destination.BankCode, tx.Destination.Name = "ANOMALY", to.Name
	return tx, nil
}

func historicalTransferRecord(tx mongorepo.TransferDocument, e journal.Event) (*journal.Record, any, error) {
	record, err := transferRecord(tx, e)
	if err != nil {
		return nil, nil, err
	}
	record.Historical = true
	return record, nil, nil
}

func sameHistoricalTransfer(tx mongorepo.TransferDocument, in HistoricalTransferInput) bool {
	return tx.ID == in.ID &&
		tx.Source.AccountID == financialID(in.SourceAccountID) &&
		tx.Destination.AccountID == financialID(in.DestinationAccountID) &&
		tx.Amount == in.Amount && tx.Fee == in.Fee && tx.Currency == in.Currency &&
		tx.Channel == in.Channel && tx.DeviceID == in.DeviceID && tx.Platform == in.Platform &&
		tx.Note == in.Note && tx.CreatedAt.Equal(in.OccurredAt)
}

func historicalTransferReference(id string) string {
	return "FT" + strings.ToUpper(strings.ReplaceAll(id, "-", ""))
}

func int64Ptr(value int64) *int64 {
	return &value
}
