package queries

import (
	"context"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type GetAccountByEmailQuery struct {
	Email string
}

type GetAccountByEmailQueryHandler struct {
	repo AccountQueryRepository
}

func NewGetAccountByEmailQueryHandler(repo AccountQueryRepository) *GetAccountByEmailQueryHandler {
	return &GetAccountByEmailQueryHandler{repo: repo}
}

func (h *GetAccountByEmailQueryHandler) Handle(ctx context.Context, q GetAccountByEmailQuery) (*accountdomain.UserAccount, error) {
	acc, err := h.repo.FindByEmail(q.Email)
	if err != nil {
		return nil, err
	}
	if acc == nil {
		return nil, accountdomain.ErrAccountNotFound
	}

	return acc, nil
}
