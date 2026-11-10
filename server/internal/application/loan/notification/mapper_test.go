package notification

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	notificationapp "github.com/dont-wait/anomaly/internal/application/notification"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/domain/notification"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
)

type fakeAccounts struct {
	account *accountdomain.UserAccount
	err     error
}

func (f fakeAccounts) FindByFinancialID(context.Context, string) (*accountdomain.UserAccount, error) {
	return f.account, f.err
}

var at = time.Date(2026, 11, 7, 3, 0, 0, 0, time.UTC)

func mapper() *Mapper {
	return NewMapper(fakeAccounts{account: &accountdomain.UserAccount{Id: "user-1", AccountNo: "1000000111"}})
}

func render(t *testing.T, r notificationapp.Request) (string, string, map[string]string) {
	t.Helper()
	tmpl, ok := notificationapp.Lookup(r.Type)
	if !ok {
		t.Fatalf("no template for %s", r.Type)
	}
	title, body, data, err := tmpl.Render(r.Params)
	if err != nil {
		t.Fatal(err)
	}
	return title, body, data
}

func TestSupports(t *testing.T) {
	m := mapper()
	if !m.Supports(journal.LoanDisbursementCredited) || !m.Supports(journal.LoanInstallmentPaid) || m.Supports(journal.Completed) {
		t.Fatal("mapper must support only loan money movements")
	}
}

func TestDisbursementNotifiesAccountOwner(t *testing.T) {
	record := journal.Record{ID: "evt-d", Type: journal.LoanDisbursementCredited, At: at, AccountID: "fin-1", LoanID: "loan-1", LoanAmount: 50000000}
	requests, err := mapper().Map(context.Background(), record)
	if err != nil || len(requests) != 1 {
		t.Fatalf("requests=%v err=%v", requests, err)
	}
	r := requests[0]
	if r.RecipientID != "user-1" || r.Type != notification.TypeLoanDisbursed || r.SourceEventID != "evt-d" || !r.OccurredAt.Equal(at) {
		t.Fatalf("wrong request: %+v", r)
	}
	title, body, data := render(t, r)
	if title != "Giải ngân thành công" || body != "+50.000.000 VND vào tài khoản •••• 0111." ||
		data["loanId"] != "loan-1" || data["amount"] != "50000000" || data["currency"] != "VND" {
		t.Fatalf("title=%q body=%q data=%v", title, body, data)
	}
}

func TestInstallmentPaidNotifiesPayer(t *testing.T) {
	record := journal.Record{ID: "evt-p", Type: journal.LoanInstallmentPaid, At: at, AccountID: "fin-1", LoanID: "loan-1",
		LoanPayment: &journal.LoanInstallmentPayment{LoanID: "loan-1", InstallmentNo: 3, Amount: 1500000}}
	requests, err := mapper().Map(context.Background(), record)
	if err != nil || len(requests) != 1 || requests[0].Type != notification.TypeLoanInstallmentPaid {
		t.Fatalf("requests=%v err=%v", requests, err)
	}
	_, body, data := render(t, requests[0])
	if body != "-1.500.000 VND từ tài khoản •••• 0111 để thanh toán kỳ 3 của khoản vay." || data["installmentNo"] != "3" || data["loanId"] != "loan-1" {
		t.Fatalf("body=%q data=%v", body, data)
	}
	if strings.Contains(body, "1000000111") {
		t.Fatal("full account number leaked")
	}
}

func TestMissingPaymentIsSkipped(t *testing.T) {
	requests, err := mapper().Map(context.Background(), journal.Record{ID: "e", Type: journal.LoanInstallmentPaid, At: at})
	if err != nil || requests != nil {
		t.Fatalf("requests=%v err=%v", requests, err)
	}
}

func TestMissingAccountAndLookupErrorsRetry(t *testing.T) {
	record := journal.Record{ID: "e", Type: journal.LoanDisbursementCredited, At: at, AccountID: "fin-1", LoanID: "l", LoanAmount: 1000}
	if _, err := NewMapper(fakeAccounts{}).Map(context.Background(), record); !errors.Is(err, errAccountNotReady) {
		t.Fatalf("missing account: %v", err)
	}
	if _, err := NewMapper(fakeAccounts{err: errors.New("mongo down")}).Map(context.Background(), record); err == nil {
		t.Fatal("lookup error must propagate")
	}
}

func TestReplayConvergesOnSameID(t *testing.T) {
	record := journal.Record{ID: "evt-d", Type: journal.LoanDisbursementCredited, At: at, AccountID: "fin-1", LoanID: "l", LoanAmount: 1000}
	a, _ := mapper().Map(context.Background(), record)
	b, _ := mapper().Map(context.Background(), record)
	if notification.NotificationID(a[0].SourceEventID, a[0].Type, a[0].RecipientID) != notification.NotificationID(b[0].SourceEventID, b[0].Type, b[0].RecipientID) {
		t.Fatal("replay must converge")
	}
}
