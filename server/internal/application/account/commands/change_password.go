package commands

import (
	"context"

	"golang.org/x/crypto/bcrypt"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

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
}

func NewChangePasswordHandler(repo AccountRepository) *ChangePasswordHandler {
	return &ChangePasswordHandler{repo: repo}
}

func (h *ChangePasswordHandler) WithLimiter(limiter PasswordAttemptLimiter) *ChangePasswordHandler {
	h.limiter = limiter
	return h
}

func (h *ChangePasswordHandler) Handle(ctx context.Context, cmd ChangePasswordCommand) error {
	if len(cmd.NewPassword) < minPasswordLength || len(cmd.NewPassword) > 72 {
		return accountdomain.ErrWeakPassword
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
	if h.limiter != nil {
		return h.limiter.Reset(ctx, cmd.AccountID)
	}
	return nil
}
