package queries

import (
	"context"

	txdomain "github.com/dont-wait/anomaly/internal/domain/transaction"
)

type ListAccountFeedQuery struct {
	AccountId string
	Limit     int
}

type ListAccountFeedQueryHandler struct {
	repo AccountFeedRepository
}

func NewListAccountFeedQueryHandler(repo AccountFeedRepository) *ListAccountFeedQueryHandler {
	return &ListAccountFeedQueryHandler{repo: repo}
}

func (h *ListAccountFeedQueryHandler) Handle(ctx context.Context, q ListAccountFeedQuery) ([]*txdomain.FeedEntry, error) {
	limit := q.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return h.repo.FindByAccountID(ctx, q.AccountId, limit)
}
