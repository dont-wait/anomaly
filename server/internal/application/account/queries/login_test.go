package queries

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/domain/audit"
	"github.com/dont-wait/anomaly/internal/domain/auth"
)

type loginRepositoryFake struct {
	account *accountdomain.UserAccount
	err     error
}

func (r *loginRepositoryFake) FindByID(context.Context, string) (*accountdomain.UserAccount, error) {
	return r.account, r.err
}

func (r *loginRepositoryFake) FindByEmail(context.Context, string) (*accountdomain.UserAccount, error) {
	return r.account, r.err
}

func (r *loginRepositoryFake) FindByUsername(context.Context, string) (*accountdomain.UserAccount, error) {
	return r.account, r.err
}

func (r *loginRepositoryFake) FindByCCCDNumber(context.Context, string) (*accountdomain.UserAccount, error) {
	return r.account, r.err
}

func (r *loginRepositoryFake) FindAll(context.Context) ([]*accountdomain.UserAccount, error) {
	if r.account == nil {
		return nil, r.err
	}
	return []*accountdomain.UserAccount{r.account}, r.err
}

type loginTokenFake struct{}

func (loginTokenFake) Issue(string, string, string, bool) (string, time.Time, error) {
	return "token", time.Now().Add(time.Hour), nil
}
func (loginTokenFake) Parse(string) (*auth.Claims, error) { return nil, nil }
func (loginTokenFake) IssueKYC(string) (string, time.Time, error) {
	return "kyc-token", time.Now().Add(time.Hour), nil
}
func (loginTokenFake) ParseKYC(string) (*auth.Claims, error) { return nil, nil }

type loginAuditFake struct {
	events []audit.Event
	err    error
}

func (r *loginAuditFake) Record(_ context.Context, event audit.Event) error {
	if r.err != nil {
		return r.err
	}
	r.events = append(r.events, event)
	return nil
}

func newLoginAccount(t *testing.T, verified bool) *accountdomain.UserAccount {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword() error = %v", err)
	}
	status := accountdomain.KYCStatusNotStarted
	if verified {
		status = accountdomain.KYCStatusVerified
	}
	return &accountdomain.UserAccount{
		Id:           "account-id",
		Username:     "alice",
		PasswordHash: string(hash),
		Customer: &accountdomain.Customer{
			Identity:  accountdomain.CustomerIdentity{Number: "cccd"},
			KYCStatus: status,
		},
	}
}

func TestLoginRecordsSuccess(t *testing.T) {
	recorder := &loginAuditFake{}
	handler := NewLoginQueryHandler(&loginRepositoryFake{account: newLoginAccount(t, true)}, loginTokenFake{}).WithAudit(recorder)

	if _, err := handler.Handle(context.Background(), LoginQuery{CCCDNumber: "cccd", Password: "password"}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(recorder.events) != 1 || recorder.events[0].Type != audit.LoginSucceeded || recorder.events[0].AccountID != "account-id" {
		t.Fatalf("audit events = %#v, want LoginSucceeded for account-id", recorder.events)
	}
}

func TestLoginRecordsInvalidCredentialsWithoutGuessingAccount(t *testing.T) {
	recorder := &loginAuditFake{}
	handler := NewLoginQueryHandler(&loginRepositoryFake{}, loginTokenFake{}).WithAudit(recorder)

	if _, err := handler.Handle(context.Background(), LoginQuery{CCCDNumber: "unknown", Password: "password"}); !errors.Is(err, accountdomain.ErrInvalidCredentials) {
		t.Fatalf("Handle() error = %v, want ErrInvalidCredentials", err)
	}
	if len(recorder.events) != 1 || recorder.events[0].Type != audit.LoginFailed || recorder.events[0].AccountID != "" {
		t.Fatalf("audit events = %#v, want account-less LoginFailed", recorder.events)
	}
}

func TestLoginRecordsKYCRequiredFailureForKnownAccount(t *testing.T) {
	recorder := &loginAuditFake{}
	handler := NewLoginQueryHandler(&loginRepositoryFake{account: newLoginAccount(t, false)}, loginTokenFake{}).WithAudit(recorder)

	if _, err := handler.Handle(context.Background(), LoginQuery{CCCDNumber: "cccd", Password: "password"}); !errors.Is(err, accountdomain.ErrKYCRequired) {
		t.Fatalf("Handle() error = %v, want ErrKYCRequired", err)
	}
	if len(recorder.events) != 1 || recorder.events[0].Type != audit.LoginFailed || recorder.events[0].AccountID != "account-id" {
		t.Fatalf("audit events = %#v, want LoginFailed for account-id", recorder.events)
	}
}
