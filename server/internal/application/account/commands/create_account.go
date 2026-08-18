package commands

import (
	"context"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type CreateAccountCommand struct {
	Username string
	Email    string
}

type CreateAccountCommandHandler struct {
	repo AccountRepository
}

func NewCreateAccountCommandHandler(repo AccountRepository) *CreateAccountCommandHandler {
	return &CreateAccountCommandHandler{repo: repo}
}

func (h *CreateAccountCommandHandler) Handle(ctx context.Context, cmd CreateAccountCommand) (*accountdomain.UserAccount, error) {
	acc := &accountdomain.UserAccount{
		Id:       newID(),
		Username: cmd.Username,
		Email:    cmd.Email,
		Amount:   0,
	}

	if err := h.repo.Save(ctx, acc); err != nil {
		return nil, err
	}

	return acc, nil
}
