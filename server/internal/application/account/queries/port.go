package queries

import (
	"context"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/domain/auth"
)

type AccountQueryRepository interface {
	FindByID(ctx context.Context, id string) (*account.UserAccount, error)
	FindByEmail(ctx context.Context, email string) (*account.UserAccount, error)
	FindByUsername(ctx context.Context, username string) (*account.UserAccount, error)
	FindAll(ctx context.Context) ([]*account.UserAccount, error)
}

// TokenService là port cho việc issue/parse JWT. Implementation cụ thể
// (HS256 với golang-jwt/v5) nằm ở infrastructure/auth.
type TokenService interface {
	Issue(userID, username string, isVerify bool) (token string, expiresAt time.Time, err error)
	Parse(tokenString string) (*auth.Claims, error)
}
