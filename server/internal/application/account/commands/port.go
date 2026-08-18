package commands

import (
	"context"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type AccountRepository interface {
	FindByID(ctx context.Context, id string) (*accountdomain.UserAccount, error)
	Save(ctx context.Context, a *accountdomain.UserAccount) error
}
