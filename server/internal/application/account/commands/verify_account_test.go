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
		IdCardFrontUrl: "kyc/account-id/id-card-front/front",
		IdCardBackUrl:  "kyc/account-id/id-card-back/back",
		LiveVideoUrl:   "kyc/account-id/live-video/live",
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

func TestVerifyAccountRejectsMediaFromAnotherAccount(t *testing.T) {
	repo := &verifyAccountRepository{account: &accountdomain.UserAccount{Id: "account-id"}}
	handler := NewVerifyAccountCommandHandler(repo)
	_, err := handler.Handle(context.Background(), VerifyAccountCommand{
		AccountID:      "account-id",
		IdCardFrontUrl: "kyc/other/id-card-front/front",
		IdCardBackUrl:  "kyc/account-id/id-card-back/back",
		LiveVideoUrl:   "kyc/account-id/live-video/live",
	})
	if err != accountdomain.ErrInvalidVerifyPayload {
		t.Fatalf("Handle() error = %v, want %v", err, accountdomain.ErrInvalidVerifyPayload)
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

func (r *verifyAccountRepository) FindByCCCDNumber(context.Context, string) (*accountdomain.UserAccount, error) {
	return nil, nil
}

func (r *verifyAccountRepository) Save(context.Context, *accountdomain.UserAccount) error {
	r.saveCalls++
	return nil
}
