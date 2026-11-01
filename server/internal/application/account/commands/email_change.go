package commands

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"

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

// EmailChangeHandler requires the current password and an OTP from the new
// address. The password stops someone holding a stolen token from moving the
// account to their own inbox, where they would receive loan-contract OTPs.
type EmailChangeHandler struct {
	repo     AccountRepository
	next     appotp.ContractOTPService
	limiter  PasswordAttemptLimiter
	pending  PendingEmailStore
	notifier EmailChangeNotifier
	ttl      time.Duration
	logger   zerolog.Logger
}

func NewEmailChangeHandler(repo AccountRepository, next appotp.ContractOTPService, limiter PasswordAttemptLimiter, pending PendingEmailStore, notifier EmailChangeNotifier, ttl time.Duration, logger zerolog.Logger) *EmailChangeHandler {
	return &EmailChangeHandler{repo: repo, next: next, limiter: limiter, pending: pending, notifier: notifier, ttl: ttl, logger: logger}
}

func (h *EmailChangeHandler) Start(ctx context.Context, accountID, newEmail, currentPassword, source string) error {
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
	// Check the password first so email availability is not exposed to token holders.
	if err := h.verifyPassword(ctx, account, currentPassword, source); err != nil {
		return err
	}
	if strings.EqualFold(account.Email, newEmail) {
		return accountdomain.ErrInvalidEmail
	}
	if err := h.ensureAvailable(ctx, accountID, newEmail); err != nil {
		return err
	}
	if _, err := h.next.Issue(ctx, accountID, newEmail); err != nil {
		return err
	}
	if err := h.pending.Set(ctx, accountID, newEmail, h.ttl); err != nil {
		return errors.Join(err, h.clear(ctx, accountID))
	}
	return nil
}

func (h *EmailChangeHandler) Confirm(ctx context.Context, accountID, newOTP string) error {
	newEmail, err := h.pending.Get(ctx, accountID)
	if err != nil {
		return err
	}
	if newEmail == "" {
		return appotp.ErrExpired
	}
	if _, err := h.next.Verify(ctx, accountID, newOTP); err != nil {
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

func (h *EmailChangeHandler) verifyPassword(ctx context.Context, account *accountdomain.UserAccount, password, source string) error {
	if h.limiter != nil {
		allowed, err := h.limiter.Allow(ctx, account.Id, source)
		if err != nil {
			return err
		}
		if !allowed {
			return accountdomain.ErrTooManyAttempts
		}
	}
	if bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(password)) != nil {
		if h.limiter != nil {
			if err := h.limiter.RecordFailure(ctx, account.Id, source); err != nil {
				return err
			}
		}
		return accountdomain.ErrCurrentPassword
	}
	if h.limiter != nil {
		if err := h.limiter.Reset(ctx, account.Id); err != nil {
			h.logger.Warn().Err(err).Msg("reset password attempts failed")
		}
	}
	return nil
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
	return errors.Join(h.next.Consume(ctx, accountID), h.pending.Delete(ctx, accountID))
}
