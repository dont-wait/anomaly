package queries

import (
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type AccountQueryRepository interface {
	FindByID(id string) (*accountdomain.UserAccount, error)
	FindByEmail(email string) (*accountdomain.UserAccount, error)
	FindAll() ([]*accountdomain.UserAccount, error)
}
