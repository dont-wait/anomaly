package commands

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"

	appotp "github.com/dont-wait/anomaly/internal/application/otp"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

// PasswordResetOTP issues challenges and redeems them exactly once. Redeeming
// leaves no authorization behind, so a concurrent request with a wrong code
// cannot succeed while the correct request is still saving.
type PasswordResetOTP interface {
	Issue(ctx context.Context, accountID, email string) (appotp.ContractOTPInfo, error)
	Redeem(ctx context.Context, accountID, code string) error
}

// PasswordResetNotifier tells the account owner about a completed reset. It is
// best effort: the new password has already committed when it runs.
type PasswordResetNotifier interface {
	NotifyPasswordReset(ctx context.Context, email string)
}

type ConfirmPasswordResetCommand struct {
	CCCDNumber  string
	OTP         string
	NewPassword string
	Source      string
}

// PasswordResetHandler recovers customer accounts by proving control of the
// registered email. Every outcome that depends on whether an account exists is
// reported identically, so these unauthenticated endpoints cannot enumerate them.
type PasswordResetHandler struct {
	repo     AccountRepository
	otp      PasswordResetOTP
	limiter  PasswordAttemptLimiter
	notifier PasswordResetNotifier
	logger   zerolog.Logger
}

func NewPasswordResetHandler(repo AccountRepository, otp PasswordResetOTP, limiter PasswordAttemptLimiter, notifier PasswordResetNotifier, logger zerolog.Logger) *PasswordResetHandler {
	return &PasswordResetHandler{repo: repo, otp: otp, limiter: limiter, notifier: notifier, logger: logger}
}

// Request sends an OTP only when the CCCD and email belong to the same customer.
// It returns the account ID when a code was sent and "" otherwise.
func (h *PasswordResetHandler) Request(ctx context.Context, cccdNumber, email string) (string, error) {
	account, err := h.findCustomer(ctx, cccdNumber)
	if err != nil {
		return "", err
	}
	if account == nil || !strings.EqualFold(account.Email, strings.TrimSpace(email)) {
		return "", nil
	}
	if _, err := h.otp.Issue(ctx, account.Id, account.Email); err != nil {
		// The code from the cooldown window is still valid.
		if errors.Is(err, appotp.ErrTooSoon) {
			return "", nil
		}
		return "", err
	}
	return account.Id, nil
}

// Confirm sets the new password and returns the account ID. Unknown accounts and
// wrong, expired or exhausted codes all return appotp.ErrInvalid. Attempts are
// counted per CCCD, so unknown accounts are throttled the same way as real ones.
func (h *PasswordResetHandler) Confirm(ctx context.Context, cmd ConfirmPasswordResetCommand) (string, error) {
	if err := validateNewPassword(cmd.NewPassword); err != nil {
		return "", err
	}
	attemptKey := resetAttemptKey(cmd.CCCDNumber)
	allowed, err := h.limiter.Allow(ctx, attemptKey, cmd.Source)
	if err != nil {
		return "", err
	}
	if !allowed {
		return "", accountdomain.ErrTooManyAttempts
	}
	account, err := h.findCustomer(ctx, cmd.CCCDNumber)
	if err != nil {
		return "", err
	}
	if account == nil {
		return "", h.rejectCode(ctx, attemptKey, cmd.Source)
	}
	if err := h.otp.Redeem(ctx, account.Id, cmd.OTP); err != nil {
		if errors.Is(err, appotp.ErrInvalid) || errors.Is(err, appotp.ErrExpired) || errors.Is(err, appotp.ErrExceeded) {
			return "", h.rejectCode(ctx, attemptKey, cmd.Source)
		}
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(cmd.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	account.PasswordHash = string(hash)
	account.TokenVersion++
	touchAccount(account)
	if err := h.repo.Save(ctx, account); err != nil {
		return "", err
	}
	// The password has committed: follow-up failures must not report it as failed.
	if err := h.limiter.Reset(ctx, attemptKey); err != nil {
		h.logger.Warn().Err(err).Msg("reset password reset attempts failed")
	}
	if h.notifier != nil {
		h.notifier.NotifyPasswordReset(ctx, account.Email)
	}
	return account.Id, nil
}

// findCustomer excludes staff accounts: email alone must not recover admin access.
func (h *PasswordResetHandler) findCustomer(ctx context.Context, cccdNumber string) (*accountdomain.UserAccount, error) {
	account, err := h.repo.FindByCCCDNumber(ctx, strings.TrimSpace(cccdNumber))
	if err != nil || account == nil || account.EffectiveRole() != accountdomain.AccountRoleUser {
		return nil, err
	}
	return account, nil
}

func (h *PasswordResetHandler) rejectCode(ctx context.Context, attemptKey, source string) error {
	if err := h.limiter.RecordFailure(ctx, attemptKey, source); err != nil {
		return err
	}
	return appotp.ErrInvalid
}

// resetAttemptKey keeps CCCD numbers out of Redis keys.
func resetAttemptKey(cccdNumber string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(cccdNumber)))
	return hex.EncodeToString(sum[:])
}
