package queries

import (
	"context"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type GetAllAccountsQuery struct{}

type GetAllAccountsQueryHandler struct {
	repo AccountQueryRepository
}

func NewGetAllAccountsQueryHandler(repo AccountQueryRepository) *GetAllAccountsQueryHandler {
	return &GetAllAccountsQueryHandler{repo: repo}
}

func (h *GetAllAccountsQueryHandler) Handle(ctx context.Context, q GetAllAccountsQuery) ([]*accountdomain.UserAccount, error) {
	return h.repo.FindAll()
}
