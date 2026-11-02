package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"

	appotp "github.com/dont-wait/anomaly/internal/application/otp"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

const (
	resetCCCD     = "079203001234"
	resetEmail    = "owner@example.com"
	resetPassword = "New-passw0rd"
)

type resetOTPStub struct {
	sentTo    string
	issueErr  error
	redeemErr error
}

func (o *resetOTPStub) Issue(_ context.Context, _, email string) (appotp.ContractOTPInfo, error) {
	if o.issueErr != nil {
		return appotp.ContractOTPInfo{}, o.issueErr
	}
	o.sentTo = email
	return appotp.ContractOTPInfo{}, nil
}

func (o *resetOTPStub) Redeem(context.Context, string, string) error { return o.redeemErr }

type resetNotifierStub struct{ email string }

func (n *resetNotifierStub) NotifyPasswordReset(_ context.Context, email string) { n.email = email }

type resetFixture struct {
	repo     *profileRepoStub
	otp      *resetOTPStub
	limiter  *limiterStub
	notifier *resetNotifierStub
	handler  *PasswordResetHandler
}

func newResetFixture(t *testing.T) *resetFixture {
	t.Helper()
	account := passwordAccount(t)
	account.Email = resetEmail
	account.Customer.Identity.Number = resetCCCD
	f := &resetFixture{
		repo:     &profileRepoStub{account: account},
		otp:      &resetOTPStub{},
		limiter:  &limiterStub{},
		notifier: &resetNotifierStub{},
	}
	f.handler = NewPasswordResetHandler(f.repo, f.otp, f.limiter, f.notifier, zerolog.Nop())
	return f
}

func (f *resetFixture) confirm(cccd string) (string, error) {
	return f.handler.Confirm(context.Background(), ConfirmPasswordResetCommand{CCCDNumber: cccd, OTP: "123456", NewPassword: resetPassword, Source: "ip"})
}

func TestPasswordResetRequestSendsOnlyWhenCCCDAndEmailMatch(t *testing.T) {
	for name, tc := range map[string]struct {
		cccd, email string
		sent        bool
	}{
		"match":          {resetCCCD, " OWNER@example.com ", true},
		"wrong email":    {resetCCCD, "other@example.com", false},
		"unknown cccd":   {"000000000000", resetEmail, false},
		"both malformed": {"x", "y", false},
	} {
		t.Run(name, func(t *testing.T) {
			f := newResetFixture(t)
			id, err := f.handler.Request(context.Background(), tc.cccd, tc.email)
			if err != nil {
				t.Fatal(err)
			}
			if tc.sent != (f.otp.sentTo == resetEmail) || tc.sent != (id != "") {
				t.Fatalf("id = %q, sentTo = %q, want sent = %v", id, f.otp.sentTo, tc.sent)
			}
		})
	}
}

func TestPasswordResetRequestSkipsStaffAccounts(t *testing.T) {
	f := newResetFixture(t)
	f.repo.account.Role = accountdomain.AccountRoleAdmin
	id, err := f.handler.Request(context.Background(), resetCCCD, resetEmail)
	if err != nil || id != "" || f.otp.sentTo != "" {
		t.Fatalf("id = %q, err = %v, sentTo = %q", id, err, f.otp.sentTo)
	}
}

func TestPasswordResetRequestHidesCooldown(t *testing.T) {
	f := newResetFixture(t)
	f.otp.issueErr = appotp.ErrTooSoon
	if id, err := f.handler.Request(context.Background(), resetCCCD, resetEmail); err != nil || id != "" {
		t.Fatalf("id = %q, err = %v", id, err)
	}
}

func TestPasswordResetConfirmReplacesHashAndInvalidatesTokens(t *testing.T) {
	f := newResetFixture(t)
	id, err := f.confirm(resetCCCD)
	if err != nil {
		t.Fatal(err)
	}
	if id != f.repo.account.Id || f.repo.account.TokenVersion != 1 || f.repo.saves != 1 {
		t.Fatalf("id = %q, account = %#v, saves = %d", id, f.repo.account, f.repo.saves)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(f.repo.account.PasswordHash), []byte(resetPassword)); err != nil {
		t.Fatalf("stored hash does not match: %v", err)
	}
	if f.limiter.resets != 1 || f.notifier.email != resetEmail {
		t.Fatalf("resets = %d, notified = %q", f.limiter.resets, f.notifier.email)
	}
}

func TestPasswordResetConfirmReportsEveryCodeFailureIdentically(t *testing.T) {
	for name, redeemErr := range map[string]error{
		"wrong":     appotp.ErrInvalid,
		"expired":   appotp.ErrExpired,
		"exhausted": appotp.ErrExceeded,
	} {
		t.Run(name, func(t *testing.T) {
			f := newResetFixture(t)
			f.otp.redeemErr = redeemErr
			_, err := f.confirm(resetCCCD)
			if !errors.Is(err, appotp.ErrInvalid) || f.limiter.failures != 1 || f.repo.saves != 0 {
				t.Fatalf("err = %v, failures = %d, saves = %d", err, f.limiter.failures, f.repo.saves)
			}
		})
	}
	t.Run("unknown account", func(t *testing.T) {
		f := newResetFixture(t)
		_, err := f.confirm("000000000000")
		if !errors.Is(err, appotp.ErrInvalid) || f.limiter.failures != 1 {
			t.Fatalf("err = %v, failures = %d", err, f.limiter.failures)
		}
	})
}

func TestPasswordResetConfirmThrottlesEvenWithCorrectCode(t *testing.T) {
	f := newResetFixture(t)
	f.limiter.blocked = true
	if _, err := f.confirm(resetCCCD); !errors.Is(err, accountdomain.ErrTooManyAttempts) || f.repo.saves != 0 {
		t.Fatalf("err = %v, saves = %d", err, f.repo.saves)
	}
}

func TestPasswordResetConfirmKeepsCodeWhenPasswordIsWeak(t *testing.T) {
	f := newResetFixture(t)
	f.otp.redeemErr = errors.New("must not redeem")
	_, err := f.handler.Confirm(context.Background(), ConfirmPasswordResetCommand{CCCDNumber: resetCCCD, OTP: "123456", NewPassword: "weakpassword"})
	if !errors.Is(err, accountdomain.ErrWeakPassword) || f.limiter.failures != 0 {
		t.Fatalf("err = %v, failures = %d", err, f.limiter.failures)
	}
}

func TestValidateNewPasswordMatchesClientRules(t *testing.T) {
	long := "Aa1!"
	for len(long) <= maxPasswordBytes {
		long += "x"
	}
	for password, valid := range map[string]bool{
		"New-passw0rd": true, "Short1!": false, "nouppercase1!": false, "NOLOWERCASE1!": false,
		"NoDigitsHere!": false, "NoSpecial123": false, long: false,
	} {
		if got := validateNewPassword(password) == nil; got != valid {
			t.Errorf("%q: valid = %v, want %v", password, got, valid)
		}
	}
}
