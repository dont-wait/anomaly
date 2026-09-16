package queries

import (
	"context"

	txdomain "github.com/dont-wait/anomaly/internal/domain/transaction"
)

type AccountFeedRepository interface {
	FindByAccountID(ctx context.Context, accountId string, limit int) ([]*txdomain.FeedEntry, error)
}
