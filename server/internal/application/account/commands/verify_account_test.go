package commands

import (
	"context"
	"testing"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

func TestVerifyAccountIsIdempotentForVerifiedAccount(t *testing.T) {
	repo := &verifyAccountRepository{
		account: &accountdomain.UserAccount{
			Id:         "account-id",
			CustomerId: "customer-id",
			Version:    1,
			Customer: &accountdomain.Customer{
				Id:        "customer-id",
				KYCStatus: accountdomain.KYCStatusNotStarted,
			},
		},
	}
	handler := NewVerifyAccountCommandHandler(repo)
	cmd := VerifyAccountCommand{
		AccountID:      repo.account.Id,
		IdCardFrontUrl: "front.jpg",
		IdCardBackUrl:  "back.jpg",
		LiveVideoUrl:   "live.mp4",
	}

	first, err := handler.Handle(context.Background(), cmd)
	if err != nil {
		t.Fatalf("first Handle() error = %v", err)
	}
	verifiedSessionID := first.Customer.VerifiedKYCSessionId
	version := first.Version

	second, err := handler.Handle(context.Background(), cmd)
	if err != nil {
		t.Fatalf("second Handle() error = %v", err)
	}

	if second != first {
		t.Error("second Handle() did not return the existing account")
	}
	if repo.saveCalls != 1 {
		t.Errorf("Save() calls = %d, want 1", repo.saveCalls)
	}
	if len(second.KYCSessions) != 1 {
		t.Errorf("KYC session count = %d, want 1", len(second.KYCSessions))
	}
	if second.Customer.VerifiedKYCSessionId != verifiedSessionID {
		t.Errorf(
			"verified KYC session ID = %q, want %q",
			second.Customer.VerifiedKYCSessionId,
			verifiedSessionID,
		)
	}
	if second.Version != version {
		t.Errorf("account version = %d, want %d", second.Version, version)
	}
}

type verifyAccountRepository struct {
	account   *accountdomain.UserAccount
	saveCalls int
}

func (r *verifyAccountRepository) FindByID(context.Context, string) (*accountdomain.UserAccount, error) {
	return r.account, nil
}

func (r *verifyAccountRepository) FindByEmail(context.Context, string) (*accountdomain.UserAccount, error) {
	return nil, nil
}

func (r *verifyAccountRepository) FindByUsername(context.Context, string) (*accountdomain.UserAccount, error) {
	return nil, nil
}

func (r *verifyAccountRepository) Save(context.Context, *accountdomain.UserAccount) error {
	r.saveCalls++
	return nil
}
