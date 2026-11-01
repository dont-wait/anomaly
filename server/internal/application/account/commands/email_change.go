package commands

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/rs/zerolog"

	appotp "github.com/dont-wait/anomaly/internal/application/otp"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

// PendingEmailStore keeps the unverified address outside the account, so it can
// never be used for login, notifications, or sensitive OTP delivery.
type PendingEmailStore interface {
	Set(ctx context.Context, accountID, email string, ttl time.Duration) error
	Get(ctx context.Context, accountID string) (string, error)
	Delete(ctx context.Context, accountID string) error
}

// EmailChangeNotifier tells the previous address about a completed change. It is
// best effort: the change has already committed when it runs.
type EmailChangeNotifier interface {
	NotifyEmailChanged(ctx context.Context, previous, next string)
}

// EmailChangeHandler requires an OTP from both addresses: verifying only the new
// one would let someone holding a stolen token move the account to their inbox.
type EmailChangeHandler struct {
	repo     AccountRepository
	current  appotp.ContractOTPService
	next     appotp.ContractOTPService
	pending  PendingEmailStore
	notifier EmailChangeNotifier
	ttl      time.Duration
	logger   zerolog.Logger
}

func NewEmailChangeHandler(repo AccountRepository, current, next appotp.ContractOTPService, pending PendingEmailStore, notifier EmailChangeNotifier, ttl time.Duration, logger zerolog.Logger) *EmailChangeHandler {
	return &EmailChangeHandler{repo: repo, current: current, next: next, pending: pending, notifier: notifier, ttl: ttl, logger: logger}
}

func (h *EmailChangeHandler) Start(ctx context.Context, accountID, newEmail string) error {
	newEmail = strings.TrimSpace(newEmail)
	parsed, err := mail.ParseAddress(newEmail)
	if err != nil || parsed.Address != newEmail {
		return accountdomain.ErrInvalidEmail
	}
	account, err := h.repo.FindByID(ctx, accountID)
	if err != nil {
		return err
	}
	if account == nil {
		return accountdomain.ErrAccountNotFound
	}
	if strings.EqualFold(account.Email, newEmail) {
		return accountdomain.ErrInvalidEmail
	}
	if err := h.ensureAvailable(ctx, accountID, newEmail); err != nil {
		return err
	}
	if _, err := h.current.Issue(ctx, accountID, account.Email); err != nil {
		return err
	}
	if _, err := h.next.Issue(ctx, accountID, newEmail); err != nil {
		return errors.Join(err, h.current.Consume(ctx, accountID))
	}
	if err := h.pending.Set(ctx, accountID, newEmail, h.ttl); err != nil {
		return errors.Join(err, h.clear(ctx, accountID))
	}
	return nil
}

func (h *EmailChangeHandler) Confirm(ctx context.Context, accountID, currentOTP, newOTP string) error {
	newEmail, err := h.pending.Get(ctx, accountID)
	if err != nil {
		return err
	}
	if newEmail == "" {
		return appotp.ErrExpired
	}
	if err := h.verify(ctx, accountID, currentOTP, newOTP); err != nil {
		if errors.Is(err, appotp.ErrExceeded) {
			return errors.Join(err, h.clear(ctx, accountID))
		}
		return err
	}
	if err := h.ensureAvailable(ctx, accountID, newEmail); err != nil {
		return err
	}
	account, err := h.repo.FindByID(ctx, accountID)
	if err != nil {
		return err
	}
	if account == nil || account.Customer == nil {
		return accountdomain.ErrAccountNotFound
	}
	previous := account.Email
	account.Email = newEmail
	account.Customer.Profile.Email = newEmail
	touchAccount(account)
	if err := h.repo.Save(ctx, account); err != nil {
		return err
	}
	if h.notifier != nil {
		h.notifier.NotifyEmailChanged(ctx, previous, newEmail)
	}
	// The change has committed: leftovers expire with their TTL, so a cleanup
	// failure must not report it as failed.
	if err := h.clear(ctx, accountID); err != nil {
		h.logger.Warn().Err(err).Msg("clear email change state failed")
	}
	return nil
}

func (h *EmailChangeHandler) verify(ctx context.Context, accountID, currentOTP, newOTP string) error {
	if _, err := h.current.Verify(ctx, accountID, currentOTP); err != nil {
		return err
	}
	_, err := h.next.Verify(ctx, accountID, newOTP)
	return err
}

func (h *EmailChangeHandler) ensureAvailable(ctx context.Context, accountID, email string) error {
	existing, err := h.repo.FindByEmail(ctx, email)
	if err != nil {
		return err
	}
	if existing != nil && existing.Id != accountID {
		return accountdomain.ErrUserAlreadyExists
	}
	return nil
}

func (h *EmailChangeHandler) clear(ctx context.Context, accountID string) error {
	return errors.Join(h.current.Consume(ctx, accountID), h.next.Consume(ctx, accountID), h.pending.Delete(ctx, accountID))
}
