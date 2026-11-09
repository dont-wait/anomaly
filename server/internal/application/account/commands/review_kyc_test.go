package commands_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/application/account/commands"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type reviewRepo struct {
	account *accountdomain.UserAccount
	saved   bool
}

func (r *reviewRepo) FindByID(_ context.Context, id string) (*accountdomain.UserAccount, error) {
	if r.account == nil || r.account.Id != id {
		return nil, nil
	}
	return r.account, nil
}

func (r *reviewRepo) FindByEmail(context.Context, string) (*accountdomain.UserAccount, error) {
	return nil, nil
}

func (r *reviewRepo) FindByUsername(context.Context, string) (*accountdomain.UserAccount, error) {
	return nil, nil
}

func (r *reviewRepo) FindByCCCDNumber(context.Context, string) (*accountdomain.UserAccount, error) {
	return nil, nil
}

func (r *reviewRepo) Save(_ context.Context, account *accountdomain.UserAccount) error {
	r.account = account
	r.saved = true
	return nil
}

func pendingAccount() *accountdomain.UserAccount {
	now := time.Now().UTC()
	return &accountdomain.UserAccount{
		Id:         "account-id",
		CustomerId: "customer-id",
		Customer: &accountdomain.Customer{
			Id:        "customer-id",
			KYCStatus: accountdomain.KYCStatusPending,
			CreatedAt: now,
		},
		KYCSessions: []*accountdomain.KYCSession{{
			Id:        "session-id",
			AttemptNo: 1,
			Status:    accountdomain.KYCSessionStatusPending,
			Media: accountdomain.KYCMedia{
				IdentityFront: accountdomain.MediaObject{StorageKey: "kyc/account-id/id-card-front/a.png"},
				IdentityBack:  accountdomain.MediaObject{StorageKey: "kyc/account-id/id-card-back/b.png"},
				LivenessVideo: accountdomain.LivenessVideo{MediaObject: accountdomain.MediaObject{StorageKey: "kyc/account-id/live-video/c.mp4"}},
			},
			CreatedAt: now,
		}},
	}
}

func TestReviewKYCApprovesPendingSession(t *testing.T) {
	repo := &reviewRepo{account: pendingAccount()}
	handler := commands.NewReviewKYCCommandHandler(repo)

	account, err := handler.Decide(context.Background(), commands.ReviewKYCCommand{
		AdminID: "admin-id", AccountID: "account-id", Decision: accountdomain.KYCReviewApproved,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !repo.saved {
		t.Fatal("account was not saved")
	}
	if account.Customer.KYCStatus != accountdomain.KYCStatusVerified {
		t.Fatalf("KYC status = %q, want verified", account.Customer.KYCStatus)
	}
	if account.Customer.VerifiedKYCSessionId != "session-id" {
		t.Fatalf("verified session = %q, want session-id", account.Customer.VerifiedKYCSessionId)
	}
	session := account.KYCSessions[0]
	if session.Status != accountdomain.KYCSessionStatusVerified || session.Review == nil {
		t.Fatalf("session = %+v, want verified with review", session)
	}
	if session.Review.Decision != accountdomain.KYCReviewApproved || session.Review.ReviewedBy != "admin-id" {
		t.Fatalf("review = %+v, want admin approval", session.Review)
	}
}

func TestReviewKYCRejectsWithReason(t *testing.T) {
	repo := &reviewRepo{account: pendingAccount()}
	handler := commands.NewReviewKYCCommandHandler(repo)

	account, err := handler.Decide(context.Background(), commands.ReviewKYCCommand{
		AdminID: "admin-id", AccountID: "account-id", Decision: accountdomain.KYCReviewRejected,
		Reason: "Ảnh CCCD bị mờ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if account.Customer.KYCStatus != accountdomain.KYCStatusRejected {
		t.Fatalf("KYC status = %q, want rejected", account.Customer.KYCStatus)
	}
	if account.Customer.VerifiedKYCSessionId != "" {
		t.Fatalf("verified session = %q, want empty", account.Customer.VerifiedKYCSessionId)
	}
	if account.KYCSessions[0].Status != accountdomain.KYCSessionStatusRejected || account.KYCSessions[0].Review.Reason != "Ảnh CCCD bị mờ" {
		t.Fatalf("session = %+v, want rejected with reason", account.KYCSessions[0])
	}
}

func TestReviewKYCRejectsInvalidDecisions(t *testing.T) {
	for _, test := range []struct {
		name string
		cmd  commands.ReviewKYCCommand
		want error
	}{
		{
			name: "self review",
			cmd:  commands.ReviewKYCCommand{AdminID: "account-id", AccountID: "account-id", Decision: accountdomain.KYCReviewApproved},
			want: accountdomain.ErrSelfKYCReview,
		},
		{
			name: "missing reason",
			cmd:  commands.ReviewKYCCommand{AdminID: "admin-id", AccountID: "account-id", Decision: accountdomain.KYCReviewRejected},
			want: accountdomain.ErrInvalidKYCReview,
		},
		{
			name: "unknown decision",
			cmd:  commands.ReviewKYCCommand{AdminID: "admin-id", AccountID: "account-id", Decision: "MAYBE"},
			want: accountdomain.ErrInvalidKYCReview,
		},
		{
			name: "unknown session",
			cmd:  commands.ReviewKYCCommand{AdminID: "admin-id", AccountID: "account-id", SessionID: "other", Decision: accountdomain.KYCReviewApproved},
			want: accountdomain.ErrKYCReviewClosed,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &reviewRepo{account: pendingAccount()}
			handler := commands.NewReviewKYCCommandHandler(repo)
			if _, err := handler.Decide(context.Background(), test.cmd); !errors.Is(err, test.want) {
				t.Fatalf("err = %v, want %v", err, test.want)
			}
			if repo.saved {
				t.Fatal("invalid decision must not persist")
			}
		})
	}
}

func TestReviewKYCRejectsAlreadyReviewedSession(t *testing.T) {
	account := pendingAccount()
	account.Customer.KYCStatus = accountdomain.KYCStatusVerified
	account.KYCSessions[0].Status = accountdomain.KYCSessionStatusVerified
	handler := commands.NewReviewKYCCommandHandler(&reviewRepo{account: account})

	_, err := handler.Decide(context.Background(), commands.ReviewKYCCommand{
		AdminID: "admin-id", AccountID: "account-id", Decision: accountdomain.KYCReviewApproved,
	})
	if !errors.Is(err, accountdomain.ErrKYCReviewClosed) {
		t.Fatalf("err = %v, want %v", err, accountdomain.ErrKYCReviewClosed)
	}
}
