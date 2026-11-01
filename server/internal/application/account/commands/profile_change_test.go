package commands

import (
	"context"
	"errors"
	"testing"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

func submitChange(t *testing.T) (*profileRepoStub, *ProfileChangeHandler, string) {
	t.Helper()
	repo := &profileRepoStub{account: customerAccount()}
	repo.account.Customer.Profile.FullName = "Tên cũ"
	handler := NewProfileChangeHandler(repo)
	account, err := handler.Submit(context.Background(), "owner", ProfileChangeInput{FullName: " Tên mới ", ProvinceCode: "hn"})
	if err != nil {
		t.Fatal(err)
	}
	return repo, handler, account.Customer.ProfileChange.Id
}

func TestProfileChangeDoesNotApplyUntilApproved(t *testing.T) {
	repo, handler, id := submitChange(t)
	if repo.account.Customer.Profile.FullName != "Tên cũ" || repo.account.Version != 1 {
		t.Fatalf("profile changed before approval: %#v", repo.account.Customer.Profile)
	}

	if err := handler.Decide(context.Background(), "admin", "owner", id, accountdomain.ProfileChangeApproved, ""); err != nil {
		t.Fatal(err)
	}
	profile := repo.account.Customer.Profile
	if profile.FullName != "Tên mới" || profile.Address.ProvinceCode != "HN" || profile.Address.Line != "12 Nguyen Trai" {
		t.Fatalf("profile = %#v", profile)
	}
	if repo.account.Version != 2 || repo.account.Customer.ProfileChange.ReviewedBy != "admin" {
		t.Fatalf("version = %d, change = %#v", repo.account.Version, repo.account.Customer.ProfileChange)
	}
	if err := handler.Decide(context.Background(), "admin", "owner", id, accountdomain.ProfileChangeApproved, ""); !errors.Is(err, accountdomain.ErrProfileChangeClosed) {
		t.Fatalf("second decision error = %v", err)
	}
}

func TestProfileChangeRejectionRequiresReasonAndKeepsProfile(t *testing.T) {
	repo, handler, id := submitChange(t)
	if err := handler.Decide(context.Background(), "admin", "owner", id, accountdomain.ProfileChangeRejected, " "); !errors.Is(err, accountdomain.ErrInvalidProfile) {
		t.Fatalf("missing reason error = %v", err)
	}
	if err := handler.Decide(context.Background(), "admin", "owner", id, accountdomain.ProfileChangeRejected, "Sai thông tin"); err != nil {
		t.Fatal(err)
	}
	if repo.account.Customer.Profile.FullName != "Tên cũ" || repo.account.Customer.ProfileChange.RejectionReason != "Sai thông tin" {
		t.Fatalf("customer = %#v", repo.account.Customer)
	}
}

func TestProfileChangeGuards(t *testing.T) {
	_, handler, id := submitChange(t)
	if err := handler.Decide(context.Background(), "owner", "owner", id, accountdomain.ProfileChangeApproved, ""); !errors.Is(err, accountdomain.ErrSelfProfileReview) {
		t.Fatalf("self review error = %v", err)
	}
	if err := handler.Decide(context.Background(), "admin", "owner", "old-id", accountdomain.ProfileChangeApproved, ""); !errors.Is(err, accountdomain.ErrProfileChangeClosed) {
		t.Fatalf("stale request error = %v", err)
	}
	if _, err := handler.Submit(context.Background(), "owner", ProfileChangeInput{}); !errors.Is(err, accountdomain.ErrInvalidProfile) {
		t.Fatalf("empty request error = %v", err)
	}
}
