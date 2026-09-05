package commands

import (
	"context"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type AccountRepository interface {
	FindByID(ctx context.Context, id string) (*accountdomain.UserAccount, error)
	FindByEmail(ctx context.Context, email string) (*accountdomain.UserAccount, error)
	FindByUsername(ctx context.Context, username string) (*accountdomain.UserAccount, error)
	Save(ctx context.Context, a *accountdomain.UserAccount) error
}

type AccountCreator interface {
	Create(ctx context.Context, account *accountdomain.UserAccount) error
}
