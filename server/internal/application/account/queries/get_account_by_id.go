package queries

import (
	"context"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type GetAccountByIDQuery struct {
	ID string
}

type GetAccountByIDQueryHandler struct {
	repo AccountQueryRepository
}

func NewGetAccountByIDQueryHandler(repo AccountQueryRepository) *GetAccountByIDQueryHandler {
	return &GetAccountByIDQueryHandler{repo: repo}
}

func (h *GetAccountByIDQueryHandler) Handle(ctx context.Context, q GetAccountByIDQuery) (
	*accountdomain.UserAccount, error,
) {
	acc, err := h.repo.FindByID(ctx, q.ID)
	if err != nil {
		return nil, err
	}
	if acc == nil {
		return nil, accountdomain.ErrAccountNotFound
	}

	return acc, nil
}
