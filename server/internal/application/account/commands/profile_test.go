package commands

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"

	appotp "github.com/dont-wait/anomaly/internal/application/otp"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type profileRepoStub struct {
	account *accountdomain.UserAccount
	other   *accountdomain.UserAccount
	saves   int
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
	s.saves++
	return nil
}

func ptr(value string) *string { return &value }

func customerAccount() *accountdomain.UserAccount {
	account := &accountdomain.UserAccount{Id: "owner", Email: "old@example.com", Customer: &accountdomain.Customer{}}
	account.Customer.Profile.Email = account.Email
	account.Customer.Profile.Phone = "+84901234567"
	account.Customer.Profile.Address = accountdomain.Address{Line: "12 Nguyen Trai", ProvinceCode: "HCM"}
	return account
}

func TestUpdateProfileMergesAddressAndIncrementsVersion(t *testing.T) {
	repo := &profileRepoStub{account: customerAccount()}
	updated, err := NewUpdateCustomerProfileHandler(repo).Handle(context.Background(), UpdateCustomerProfileCommand{AccountID: "owner", ProvinceCode: ptr("hn")})
	if err != nil {
		t.Fatal(err)
	}
	address := updated.Customer.Profile.Address
	if address.Line != "12 Nguyen Trai" || address.ProvinceCode != "HN" {
		t.Fatalf("address not merged: %#v", address)
	}
	if updated.Customer.Profile.Phone != "+84901234567" {
		t.Fatalf("omitted phone changed: %q", updated.Customer.Profile.Phone)
	}
	// BankRepository.Save persists nothing when Version is unchanged.
	if updated.Version != 1 || repo.saves != 1 {
		t.Fatalf("version = %d, saves = %d", updated.Version, repo.saves)
	}
}

func TestUpdateProfileRejectsEmptyValuesAndEmptyRequest(t *testing.T) {
	for name, cmd := range map[string]UpdateCustomerProfileCommand{
		"no fields":      {AccountID: "owner"},
		"empty phone":    {AccountID: "owner", Phone: ptr(" ")},
		"invalid phone":  {AccountID: "owner", Phone: ptr("abc")},
		"empty line":     {AccountID: "owner", AddressLine: ptr("")},
		"empty province": {AccountID: "owner", ProvinceCode: ptr("")},
	} {
		repo := &profileRepoStub{account: customerAccount()}
		if _, err := NewUpdateCustomerProfileHandler(repo).Handle(context.Background(), cmd); !errors.Is(err, accountdomain.ErrInvalidProfile) {
			t.Errorf("%s: error = %v", name, err)
		}
		if repo.saves != 0 {
			t.Errorf("%s: saved invalid profile", name)
		}
	}
}

type limiterStub struct {
	blocked  bool
	failures int
	resets   int
	resetErr error
}

func (l *limiterStub) Allow(context.Context, string, string) (bool, error) { return !l.blocked, nil }

func (l *limiterStub) RecordFailure(context.Context, string, string) error {
	l.failures++
	return nil
}

func (l *limiterStub) Reset(context.Context, string) error {
	l.resets++
	return l.resetErr
}

func passwordAccount(t *testing.T) *accountdomain.UserAccount {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("old-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	account := customerAccount()
	account.PasswordHash = string(hash)
	return account
}

func TestChangePasswordPersistsHashAndInvalidatesTokens(t *testing.T) {
	repo := &profileRepoStub{account: passwordAccount(t)}
	limiter := &limiterStub{}
	err := NewChangePasswordHandler(repo).WithLimiter(limiter, zerolog.Nop()).Handle(context.Background(), ChangePasswordCommand{AccountID: "owner", CurrentPassword: "old-password", NewPassword: "new-password"})
	if err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(repo.account.PasswordHash), []byte("new-password")); err != nil {
		t.Fatalf("stored hash does not match: %v", err)
	}
	if repo.account.TokenVersion != 1 || repo.account.Version != 1 || repo.saves != 1 || limiter.resets != 1 {
		t.Fatalf("account = %#v, saves = %d, resets = %d", repo.account, repo.saves, limiter.resets)
	}
}

func TestChangePasswordRecordsWrongCurrentPassword(t *testing.T) {
	repo := &profileRepoStub{account: passwordAccount(t)}
	limiter := &limiterStub{}
	err := NewChangePasswordHandler(repo).WithLimiter(limiter, zerolog.Nop()).Handle(context.Background(), ChangePasswordCommand{AccountID: "owner", CurrentPassword: "wrong", NewPassword: "new-password"})
	if !errors.Is(err, accountdomain.ErrCurrentPassword) {
		t.Fatalf("error = %v", err)
	}
	if limiter.failures != 1 || repo.saves != 0 {
		t.Fatalf("failures = %d, saves = %d", limiter.failures, repo.saves)
	}
}

func TestChangePasswordThrottledEvenWithCorrectPassword(t *testing.T) {
	repo := &profileRepoStub{account: passwordAccount(t)}
	err := NewChangePasswordHandler(repo).WithLimiter(&limiterStub{blocked: true}, zerolog.Nop()).Handle(context.Background(), ChangePasswordCommand{AccountID: "owner", CurrentPassword: "old-password", NewPassword: "new-password"})
	if !errors.Is(err, accountdomain.ErrTooManyAttempts) || repo.saves != 0 {
		t.Fatalf("error = %v, saves = %d", err, repo.saves)
	}
}

type otpStub struct {
	code     string
	sentTo   string
	issueErr error
	consumed int
}

func (o *otpStub) Issue(_ context.Context, _, email string) (appotp.ContractOTPInfo, error) {
	if o.issueErr != nil {
		return appotp.ContractOTPInfo{}, o.issueErr
	}
	o.sentTo = email
	return appotp.ContractOTPInfo{}, nil
}

func (o *otpStub) Verify(_ context.Context, _, code string) (int, error) {
	if code != o.code {
		return 0, appotp.ErrInvalid
	}
	return 3, nil
}

func (o *otpStub) Consume(context.Context, string) error {
	o.consumed++
	return nil
}

type pendingStub struct {
	email     string
	deleteErr error
}

func (p *pendingStub) Set(_ context.Context, _, email string, _ time.Duration) error {
	p.email = email
	return nil
}
func (p *pendingStub) Get(context.Context, string) (string, error) { return p.email, nil }
func (p *pendingStub) Delete(context.Context, string) error {
	p.email = ""
	return p.deleteErr
}

type notifierStub struct{ previous string }

func (n *notifierStub) NotifyEmailChanged(_ context.Context, previous, _ string) {
	n.previous = previous
}

type emailFixture struct {
	repo     *profileRepoStub
	next     *otpStub
	limiter  *limiterStub
	pending  *pendingStub
	notifier *notifierStub
	handler  *EmailChangeHandler
}

func newEmailFixture(t *testing.T) *emailFixture {
	t.Helper()
	f := &emailFixture{repo: &profileRepoStub{account: passwordAccount(t)}, next: &otpStub{code: "222222"}, limiter: &limiterStub{}, pending: &pendingStub{}, notifier: &notifierStub{}}
	f.handler = NewEmailChangeHandler(f.repo, f.next, f.limiter, f.pending, f.notifier, time.Minute, zerolog.Nop())
	return f
}

func (f *emailFixture) start(email, password string) error {
	return f.handler.Start(context.Background(), "owner", email, password, "1.2.3.4")
}

func TestEmailChangeStartKeepsActiveEmailAndSendsOnlyToNewAddress(t *testing.T) {
	f := newEmailFixture(t)
	if err := f.start("new@example.com", "old-password"); err != nil {
		t.Fatal(err)
	}
	if f.repo.account.Email != "old@example.com" || f.repo.saves != 0 {
		t.Fatalf("email activated before verification: %q", f.repo.account.Email)
	}
	if f.next.sentTo != "new@example.com" || f.pending.email != "new@example.com" || f.limiter.resets != 1 {
		t.Fatalf("sent = %q, pending = %q, resets = %d", f.next.sentTo, f.pending.email, f.limiter.resets)
	}
}

func TestEmailChangeStartRequiresCurrentPassword(t *testing.T) {
	f := newEmailFixture(t)
	if err := f.start("new@example.com", "wrong"); !errors.Is(err, accountdomain.ErrCurrentPassword) {
		t.Fatalf("error = %v", err)
	}
	if f.next.sentTo != "" || f.pending.email != "" || f.limiter.failures != 1 {
		t.Fatalf("sent = %q, pending = %q, failures = %d", f.next.sentTo, f.pending.email, f.limiter.failures)
	}

	f.limiter.blocked = true
	if err := f.start("new@example.com", "old-password"); !errors.Is(err, accountdomain.ErrTooManyAttempts) {
		t.Fatalf("throttled error = %v", err)
	}
	if f.next.sentTo != "" {
		t.Fatal("OTP sent while throttled")
	}
}

func TestEmailChangeStartRejectsDuplicateAndSameEmail(t *testing.T) {
	f := newEmailFixture(t)
	f.repo.other = &accountdomain.UserAccount{Id: "other", Email: "used@example.com"}
	if err := f.start("used@example.com", "wrong"); !errors.Is(err, accountdomain.ErrCurrentPassword) {
		t.Fatalf("availability exposed before the password check: %v", err)
	}
	if err := f.start("used@example.com", "old-password"); !errors.Is(err, accountdomain.ErrUserAlreadyExists) {
		t.Fatalf("duplicate error = %v", err)
	}
	if err := f.start("old@example.com", "old-password"); !errors.Is(err, accountdomain.ErrInvalidEmail) {
		t.Fatalf("same email error = %v", err)
	}
	if f.next.sentTo != "" {
		t.Fatal("OTP sent for a rejected address")
	}
}

func TestEmailChangeConfirmRejectsWrongCode(t *testing.T) {
	f := newEmailFixture(t)
	f.pending.email = "new@example.com"
	if err := f.handler.Confirm(context.Background(), "owner", "000000"); !errors.Is(err, appotp.ErrInvalid) {
		t.Fatalf("error = %v", err)
	}
	if f.repo.account.Email != "old@example.com" || f.repo.saves != 0 {
		t.Fatal("email changed with a wrong code")
	}
}

func TestEmailChangeConfirmUpdatesBothCopiesAndNotifies(t *testing.T) {
	f := newEmailFixture(t)
	f.pending.email = "new@example.com"
	if err := f.handler.Confirm(context.Background(), "owner", "222222"); err != nil {
		t.Fatal(err)
	}
	account := f.repo.account
	if account.Email != "new@example.com" || account.Customer.Profile.Email != "new@example.com" {
		t.Fatalf("email copies = %q, %q", account.Email, account.Customer.Profile.Email)
	}
	if account.Version != 1 || f.repo.saves != 1 || f.pending.email != "" || f.notifier.previous != "old@example.com" {
		t.Fatalf("version = %d, saves = %d, pending = %q, notified = %q", account.Version, f.repo.saves, f.pending.email, f.notifier.previous)
	}
}

func TestEmailChangeConfirmSucceedsWhenCleanupFails(t *testing.T) {
	f := newEmailFixture(t)
	f.pending.email = "new@example.com"
	f.pending.deleteErr = errors.New("redis down")
	if err := f.handler.Confirm(context.Background(), "owner", "222222"); err != nil {
		t.Fatalf("error = %v", err)
	}
	if f.repo.account.Email != "new@example.com" {
		t.Fatalf("email = %q", f.repo.account.Email)
	}
}

func TestEmailChangeConfirmWithoutPendingExpires(t *testing.T) {
	f := newEmailFixture(t)
	if err := f.handler.Confirm(context.Background(), "owner", "222222"); !errors.Is(err, appotp.ErrExpired) {
		t.Fatalf("error = %v", err)
	}
}

func TestChangePasswordSucceedsWhenResetFails(t *testing.T) {
	repo := &profileRepoStub{account: passwordAccount(t)}
	limiter := &limiterStub{resetErr: errors.New("redis down")}
	err := NewChangePasswordHandler(repo).WithLimiter(limiter, zerolog.Nop()).Handle(context.Background(), ChangePasswordCommand{AccountID: "owner", CurrentPassword: "old-password", NewPassword: "new-password"})
	if err != nil || repo.saves != 1 {
		t.Fatalf("error = %v, saves = %d", err, repo.saves)
	}
}
