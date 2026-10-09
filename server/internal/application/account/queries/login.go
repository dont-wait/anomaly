package queries

import (
	"context"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type LoginQuery struct {
	CCCDNumber string
	Password   string
}

type LoginResult struct {
	User      *accountdomain.UserAccount
	Token     string
	ExpiresAt time.Time
}

type KYCSessionResult struct {
	User         *accountdomain.UserAccount
	KYCToken     string
	KYCExpiresAt time.Time
}

type LoginQueryHandler struct {
	readRepo AccountQueryRepository
	tokens   TokenService
}

func NewLoginQueryHandler(readRepo AccountQueryRepository, tokens TokenService) *LoginQueryHandler {
	return &LoginQueryHandler{readRepo: readRepo, tokens: tokens}
}

func (h *LoginQueryHandler) Handle(ctx context.Context, q LoginQuery) (*LoginResult, error) {
	acc, err := validateCredentials(ctx, h.readRepo, q.CCCDNumber, q.Password)
	if err != nil {
		return nil, err
	}
	if !acc.IsVerified() {
		return nil, accountdomain.ErrKYCRequired
	}

	token, expiresAt, err := h.tokens.Issue(acc.Id, acc.Username, string(acc.EffectiveRole()), acc.IsVerified())
	if err != nil {
		return nil, err
	}

	return &LoginResult{
		User:      acc,
		Token:     token,
		ExpiresAt: expiresAt,
	}, nil
}

func (h *LoginQueryHandler) StartKYCSession(ctx context.Context, q LoginQuery) (*KYCSessionResult, error) {
	acc, err := validateCredentials(ctx, h.readRepo, q.CCCDNumber, q.Password)
	if err != nil {
		return nil, err
	}
	if acc.IsVerified() {
		return nil, accountdomain.ErrAccountAlreadyVerified
	}
	token, expiresAt, err := h.tokens.IssueKYC(acc.Id)
	if err != nil {
		return nil, err
	}
	return &KYCSessionResult{User: acc, KYCToken: token, KYCExpiresAt: expiresAt}, nil
}

func validateCredentials(ctx context.Context, repo AccountQueryRepository, cccdNumber, password string) (*accountdomain.UserAccount, error) {
	acc, err := repo.FindByCCCDNumber(ctx, cccdNumber)
	if err != nil {
		return nil, err
	}
	if acc == nil {
		return nil, accountdomain.ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(acc.PasswordHash), []byte(password)); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return nil, accountdomain.ErrInvalidCredentials
		}
		return nil, accountdomain.ErrInvalidCredentials
	}
	return acc, nil
}
