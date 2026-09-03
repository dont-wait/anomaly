package queries

import (
	"context"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type LoginQuery struct {
	Login    string
	Password string
}

type LoginResult struct {
	User      *accountdomain.UserAccount
	Token     string
	ExpiresAt time.Time
}

type LoginQueryHandler struct {
	readRepo AccountQueryRepository
	tokens   TokenService
}

func NewLoginQueryHandler(readRepo AccountQueryRepository, tokens TokenService) *LoginQueryHandler {
	return &LoginQueryHandler{readRepo: readRepo, tokens: tokens}
}

func (h *LoginQueryHandler) Handle(ctx context.Context, q LoginQuery) (*LoginResult, error) {
	acc, err := h.readRepo.FindByEmail(ctx, q.Login)
	if err != nil {
		return nil, err
	}
	if acc == nil {
		acc, err = h.readRepo.FindByUsername(ctx, q.Login)
		if err != nil {
			return nil, err
		}
	}
	if acc == nil {
		return nil, accountdomain.ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword(
		[]byte(acc.PasswordHash),
		[]byte(q.Password)); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return nil, accountdomain.ErrInvalidCredentials
		}
		return nil, accountdomain.ErrInvalidCredentials
	}

	token, expiresAt, err := h.tokens.Issue(acc.Id, acc.Username, acc.IsVerify)
	if err != nil {
		return nil, err
	}

	return &LoginResult{
		User:      acc,
		Token:     token,
		ExpiresAt: expiresAt,
	}, nil
}
