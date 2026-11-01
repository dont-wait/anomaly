package queries

import (
	"context"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/domain/audit"
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
	readRepo    AccountQueryRepository
	credentials CredentialRepository
	tokens      TokenService
	audit       audit.Recorder
}

func (h *LoginQueryHandler) WithCredentials(repo CredentialRepository) *LoginQueryHandler {
	h.credentials = repo
	return h
}

func NewLoginQueryHandler(readRepo AccountQueryRepository, tokens TokenService) *LoginQueryHandler {
	return &LoginQueryHandler{readRepo: readRepo, tokens: tokens}
}

func (h *LoginQueryHandler) WithAudit(recorder audit.Recorder) *LoginQueryHandler {
	h.audit = recorder
	return h
}

func (h *LoginQueryHandler) Handle(ctx context.Context, q LoginQuery) (*LoginResult, error) {
	acc, err := validateCredentials(ctx, h.readRepo, h.credentials, q.CCCDNumber, q.Password)
	if err != nil {
		if errors.Is(err, accountdomain.ErrInvalidCredentials) {
			if auditErr := h.recordLogin(ctx, audit.LoginFailed, "invalid_credentials", ""); auditErr != nil {
				return nil, auditErr
			}
		}
		return nil, err
	}
	if !acc.IsVerified() {
		if auditErr := h.recordLogin(ctx, audit.LoginFailed, "kyc_required", acc.Id); auditErr != nil {
			return nil, auditErr
		}
		return nil, accountdomain.ErrKYCRequired
	}

	token, expiresAt, err := h.tokens.Issue(acc.Id, acc.Username, string(acc.EffectiveRole()), acc.IsVerified())
	if err != nil {
		return nil, err
	}
	if err := h.recordLogin(ctx, audit.LoginSucceeded, "", acc.Id); err != nil {
		return nil, err
	}

	return &LoginResult{
		User:      acc,
		Token:     token,
		ExpiresAt: expiresAt,
	}, nil
}

func (h *LoginQueryHandler) recordLogin(ctx context.Context, eventType, reasonCode, accountID string) error {
	if h.audit == nil {
		return nil
	}
	event := audit.NewEvent(eventType, "", time.Now().UTC())
	event.AccountID = accountID
	event.ActorID = accountID
	event.ReasonCode = reasonCode
	return h.audit.Record(ctx, event)
}

func (h *LoginQueryHandler) StartKYCSession(ctx context.Context, q LoginQuery) (*KYCSessionResult, error) {
	acc, err := validateCredentials(ctx, h.readRepo, h.credentials, q.CCCDNumber, q.Password)
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

func validateCredentials(ctx context.Context, repo AccountQueryRepository, credentials CredentialRepository, cccdNumber, password string) (*accountdomain.UserAccount, error) {
	acc, err := repo.FindByCCCDNumber(ctx, cccdNumber)
	if err != nil {
		return nil, err
	}
	if acc == nil {
		return nil, accountdomain.ErrInvalidCredentials
	}
	passwordHash := acc.PasswordHash
	if credentials != nil {
		credentialAccount, err := credentials.FindByCCCDNumber(ctx, cccdNumber)
		if err != nil {
			return nil, err
		}
		if credentialAccount == nil || credentialAccount.Id != acc.Id {
			return nil, accountdomain.ErrInvalidCredentials
		}
		passwordHash = credentialAccount.PasswordHash
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return nil, accountdomain.ErrInvalidCredentials
		}
		return nil, accountdomain.ErrInvalidCredentials
	}
	return acc, nil
}
