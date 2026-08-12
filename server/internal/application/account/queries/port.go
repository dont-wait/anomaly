package queries

import (
	"context"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type AccountQueryRepository interface {
	FindByID(ctx context.Context, id string) (*accountdomain.UserAccount, error)
	FindByEmail(ctx context.Context, email string) (*accountdomain.UserAccount, error)
	FindAll(ctx context.Context) ([]*accountdomain.UserAccount, error)
}
