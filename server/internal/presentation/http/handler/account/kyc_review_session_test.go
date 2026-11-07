package account

import (
	"errors"
	"net/http"
	"testing"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
)

func reviewTestAccount(status accountdomain.KYCStatus, sessions ...*accountdomain.KYCSession) *accountdomain.UserAccount {
	return &accountdomain.UserAccount{
		Id:          "account-id",
		Customer:    &accountdomain.Customer{KYCStatus: status},
		KYCSessions: sessions,
	}
}

func TestReviewSessionServesOnlyPendingSession(t *testing.T) {
	old := &accountdomain.KYCSession{Id: "old", AttemptNo: 1, Status: accountdomain.KYCSessionStatusRejected}
	pending := &accountdomain.KYCSession{Id: "current", AttemptNo: 2, Status: accountdomain.KYCSessionStatusPending}
	account := reviewTestAccount(accountdomain.KYCStatusPending, old, pending)

	if got := reviewSession(account, ""); got != pending {
		t.Fatalf("reviewSession(default) = %+v, want pending session", got)
	}
	if got := reviewSession(account, "current"); got != pending {
		t.Fatalf("reviewSession(current) = %+v, want pending session", got)
	}
	if got := reviewSession(account, "old"); got != nil {
		t.Fatalf("reviewSession(old decided session) = %+v, want nil", got)
	}
}

func TestReviewSessionHidesDecidedAccounts(t *testing.T) {
	verified := &accountdomain.KYCSession{Id: "done", AttemptNo: 1, Status: accountdomain.KYCSessionStatusVerified}
	account := reviewTestAccount(accountdomain.KYCStatusVerified, verified)

	if got := reviewSession(account, ""); got != nil {
		t.Fatalf("reviewSession(default) = %+v, want nil for verified account", got)
	}
	if got := reviewSession(account, "done"); got != nil {
		t.Fatalf("reviewSession(done) = %+v, want nil for verified account", got)
	}
}

func TestSelfKYCReviewHasDedicatedErrorCode(t *testing.T) {
	if got := accountErrorStatus(accountdomain.ErrSelfKYCReview); got != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", got)
	}
	if got := accountErrorCode(accountdomain.ErrSelfKYCReview); got != httpx.ErrorCodeSelfKYCReview {
		t.Fatalf("code = %q, want %q", got, httpx.ErrorCodeSelfKYCReview)
	}
	if got := accountErrorCode(errors.New("other")); got == httpx.ErrorCodeSelfKYCReview {
		t.Fatalf("unrelated error mapped to %q", got)
	}
}
