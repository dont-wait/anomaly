package commands

import (
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type AccountRepository interface {
	FindByID(id string) (*accountdomain.UserAccount, error)
	Save(a *accountdomain.UserAccount) error
}
