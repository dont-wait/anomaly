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
}

type PasswordRepository interface {
	FindByID(context.Context, string) (*accountdomain.UserAccount, error)
	UpdatePasswordHash(context.Context, string, string) error
}

type ChangePasswordHandler struct{ repo PasswordRepository }

func NewChangePasswordHandler(repo PasswordRepository) *ChangePasswordHandler {
	return &ChangePasswordHandler{repo: repo}
}

func (h *ChangePasswordHandler) Handle(ctx context.Context, cmd ChangePasswordCommand) error {
	if len(cmd.NewPassword) < minPasswordLength || len(cmd.NewPassword) > 72 {
		return accountdomain.ErrWeakPassword
	}
	account, err := h.repo.FindByID(ctx, cmd.AccountID)
	if err != nil {
		return err
	}
	if account == nil {
		return accountdomain.ErrAccountNotFound
	}
	if bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(cmd.CurrentPassword)) != nil {
		return accountdomain.ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(cmd.NewPassword)) == nil {
		return accountdomain.ErrWeakPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(cmd.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return h.repo.UpdatePasswordHash(ctx, account.Id, string(hash))
}
