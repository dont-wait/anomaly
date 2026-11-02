package commands

import (
	"context"

	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

const maxPasswordBytes = 72

type ChangePasswordCommand struct {
	AccountID       string
	CurrentPassword string
	NewPassword     string
	Source          string
}

// PasswordAttemptLimiter bounds current-password guesses per account and per source.
type PasswordAttemptLimiter interface {
	Allow(ctx context.Context, accountID, source string) (bool, error)
	RecordFailure(ctx context.Context, accountID, source string) error
	Reset(ctx context.Context, accountID string) error
}

type ChangePasswordHandler struct {
	repo    AccountRepository
	limiter PasswordAttemptLimiter
	logger  zerolog.Logger
}

func NewChangePasswordHandler(repo AccountRepository) *ChangePasswordHandler {
	return &ChangePasswordHandler{repo: repo, logger: zerolog.Nop()}
}

func (h *ChangePasswordHandler) WithLimiter(limiter PasswordAttemptLimiter, logger zerolog.Logger) *ChangePasswordHandler {
	h.limiter = limiter
	h.logger = logger
	return h
}

// validateNewPassword mirrors the client rules. bcrypt only hashes the first 72
// bytes, so longer passwords are rejected instead of being silently truncated.
func validateNewPassword(password string) error {
	if len(password) < minPasswordLength || len(password) > maxPasswordBytes {
		return accountdomain.ErrWeakPassword
	}
	var lower, upper, digit, special bool
	for _, r := range password {
		switch {
		case r >= 'a' && r <= 'z':
			lower = true
		case r >= 'A' && r <= 'Z':
			upper = true
		case r >= '0' && r <= '9':
			digit = true
		default:
			special = true
		}
	}
	if !lower || !upper || !digit || !special {
		return accountdomain.ErrWeakPassword
	}
	return nil
}

func (h *ChangePasswordHandler) Handle(ctx context.Context, cmd ChangePasswordCommand) error {
	if err := validateNewPassword(cmd.NewPassword); err != nil {
		return err
	}
	if h.limiter != nil {
		allowed, err := h.limiter.Allow(ctx, cmd.AccountID, cmd.Source)
		if err != nil {
			return err
		}
		if !allowed {
			return accountdomain.ErrTooManyAttempts
		}
	}
	account, err := h.repo.FindByID(ctx, cmd.AccountID)
	if err != nil {
		return err
	}
	if account == nil {
		return accountdomain.ErrAccountNotFound
	}
	if bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(cmd.CurrentPassword)) != nil {
		if h.limiter != nil {
			if err := h.limiter.RecordFailure(ctx, cmd.AccountID, cmd.Source); err != nil {
				return err
			}
		}
		return accountdomain.ErrCurrentPassword
	}
	if bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(cmd.NewPassword)) == nil {
		return accountdomain.ErrWeakPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(cmd.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	// The hash and token version travel in one account event, so replay restores
	// the new credential and every earlier token stops matching together.
	account.PasswordHash = string(hash)
	account.TokenVersion++
	touchAccount(account)
	if err := h.repo.Save(ctx, account); err != nil {
		return err
	}
	// The password has committed: a cleanup failure must not report it as failed.
	if h.limiter != nil {
		if err := h.limiter.Reset(ctx, cmd.AccountID); err != nil {
			h.logger.Warn().Err(err).Msg("reset password attempts failed")
		}
	}
	return nil
}
