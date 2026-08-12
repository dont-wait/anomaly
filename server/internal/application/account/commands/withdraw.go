package commands

import (
	"context"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type WithdrawCommand struct {
	AccountID string
	Amount    int64
}

type WithdrawCommandHandler struct {
	repo AccountRepository
}

func NewWithdrawCommandHandler(repo AccountRepository) *WithdrawCommandHandler {
	return &WithdrawCommandHandler{repo: repo}
}

func (h *WithdrawCommandHandler) Handle(ctx context.Context, cmd WithdrawCommand) error {
	acc, err := h.repo.FindByID(ctx, cmd.AccountID)
	if err != nil {
		return err
	}
	if acc == nil {
		return accountdomain.ErrAccountNotFound
	}

	if err := acc.Withdraw(cmd.Amount); err != nil {
		return err
	}

	return h.repo.Save(ctx, acc)
}
