package commands

import (
	"context"
	"testing"

	"golang.org/x/crypto/bcrypt"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type profileRepoStub struct {
	account      *accountdomain.UserAccount
	other        *accountdomain.UserAccount
	saved        bool
	passwordHash string
}

func (s *profileRepoStub) FindByID(_ context.Context, id string) (*accountdomain.UserAccount, error) {
	if s.account != nil && s.account.Id == id {
		return s.account, nil
	}
	return nil, nil
}
func (s *profileRepoStub) FindByEmail(_ context.Context, email string) (*accountdomain.UserAccount, error) {
	if s.account != nil && s.account.Email == email {
		return s.account, nil
	}
	if s.other != nil && s.other.Email == email {
		return s.other, nil
	}
	return nil, nil
}
func (s *profileRepoStub) FindByUsername(context.Context, string) (*accountdomain.UserAccount, error) {
	return nil, nil
}
func (s *profileRepoStub) FindByCCCDNumber(context.Context, string) (*accountdomain.UserAccount, error) {
	return nil, nil
}
func (s *profileRepoStub) Save(context.Context, *accountdomain.UserAccount) error {
	s.saved = true
	return nil
}
func (s *profileRepoStub) UpdatePasswordHash(_ context.Context, _ string, hash string) error {
	s.passwordHash = hash
	return nil
}

func TestUpdateCustomerProfileChangesOnlyAllowedFields(t *testing.T) {
	repo := &profileRepoStub{account: &accountdomain.UserAccount{Id: "owner", Email: "old@example.com", Version: 2, Customer: &accountdomain.Customer{Profile: accountdomain.CustomerProfile{FullName: "Verified Name"}, Identity: accountdomain.CustomerIdentity{Number: "123456789012"}}}}
	phone, email := "+84901234567", "new@example.com"
	address := &accountdomain.Address{Line: "12 Main Street", ProvinceCode: "hcm"}
	updated, err := NewUpdateCustomerProfileHandler(repo).Handle(context.Background(), UpdateCustomerProfileCommand{AccountID: "owner", Phone: &phone, Email: &email, Address: address})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Customer.Profile.FullName != "Verified Name" || updated.Customer.Identity.Number != "123456789012" {
		t.Fatal("verified identity fields were changed")
	}
	if updated.Customer.Profile.Phone != phone || updated.Email != email || updated.Customer.Profile.Address.ProvinceCode != "HCM" || !repo.saved {
		t.Fatalf("profile not updated: %#v", updated)
	}
}

func TestUpdateCustomerProfileRejectsDuplicateEmail(t *testing.T) {
	owner := &accountdomain.UserAccount{Id: "owner", Email: "old@example.com", Customer: &accountdomain.Customer{}}
	other := &accountdomain.UserAccount{Id: "other", Email: "used@example.com"}
	repo := &profileRepoStub{account: owner, other: other}
	_, err := NewUpdateCustomerProfileHandler(repo).Handle(context.Background(), UpdateCustomerProfileCommand{AccountID: "owner", Email: ptr("used@example.com")})
	if err != accountdomain.ErrUserAlreadyExists {
		t.Fatalf("duplicate email error = %v", err)
	}
}

func TestChangePasswordVerifiesCurrentAndStoresOnlyHash(t *testing.T) {
	oldHash, err := bcrypt.GenerateFromPassword([]byte("old-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	repo := &profileRepoStub{account: &accountdomain.UserAccount{Id: "owner", PasswordHash: string(oldHash)}}
	handler := NewChangePasswordHandler(repo)
	if err := handler.Handle(context.Background(), ChangePasswordCommand{AccountID: "owner", CurrentPassword: "wrong-password", NewPassword: "new-password"}); err != accountdomain.ErrInvalidCredentials {
		t.Fatalf("wrong current password error = %v", err)
	}
	if repo.passwordHash != "" {
		t.Fatal("password changed after wrong current password")
	}
	if err := handler.Handle(context.Background(), ChangePasswordCommand{AccountID: "owner", CurrentPassword: "old-password", NewPassword: "new-password"}); err != nil {
		t.Fatal(err)
	}
	if repo.passwordHash == "new-password" {
		t.Fatal("plain password was stored")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(repo.passwordHash), []byte("new-password")); err != nil {
		t.Fatalf("stored hash does not match new password: %v", err)
	}
}

func ptr(value string) *string { return &value }
