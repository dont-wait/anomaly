package commands

import (
	"context"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type VerifyAccountCommand struct {
	AccountID      string
	IdCardFrontUrl string
	IdCardBackUrl  string
	LiveVideoUrl   string
}

type VerifyAccountCommandHandler struct {
	repo AccountRepository
}

func NewVerifyAccountCommandHandler(repo AccountRepository) *VerifyAccountCommandHandler {
	return &VerifyAccountCommandHandler{repo: repo}
}

func (h *VerifyAccountCommandHandler) Handle(ctx context.Context, cmd VerifyAccountCommand) (*accountdomain.UserAccount, error) {
	if cmd.IdCardFrontUrl == "" || cmd.IdCardBackUrl == "" || cmd.LiveVideoUrl == "" {
		return nil, accountdomain.ErrInvalidVerifyPayload
	}

	acc, err := h.repo.FindByID(ctx, cmd.AccountID)
	if err != nil {
		return nil, err
	}
	if acc == nil {
		return nil, accountdomain.ErrAccountNotFound
	}

	acc.IdCardFrontUrl = cmd.IdCardFrontUrl
	acc.IdCardBackUrl = cmd.IdCardBackUrl
	acc.LiveVideoUrl = cmd.LiveVideoUrl
	acc.IsVerify = true

	if err := h.repo.Save(ctx, acc); err != nil {
		return nil, err
	}

	return acc, nil
}
