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
	"github.com/dont-wait/anomaly/internal/domain/savings"
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

func opened() journal.Record {
	return journal.Record{ID: "evt-o", Type: journal.SavingsOpened, At: at, AccountID: "fin-1",
		Savings: &journal.SavingsMovement{Contract: savings.Contract{ID: "sav-1", Principal: 10000000, TermMonths: 6}}}
}

func settled() journal.Record {
	return journal.Record{ID: "evt-s", Type: journal.SavingsSettled, At: at, AccountID: "fin-1",
		Savings: &journal.SavingsMovement{Contract: savings.Contract{ID: "sav-1", Principal: 10000000,
			Settlement: &savings.Settlement{InterestAmount: 250000, TotalAmount: 10250000}}}}
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
	if !m.Supports(journal.SavingsOpened) || !m.Supports(journal.SavingsSettled) || m.Supports(journal.Completed) {
		t.Fatal("mapper must support only savings money movements")
	}
}

func TestOpenedNotifiesOwner(t *testing.T) {
	requests, err := mapper().Map(context.Background(), opened())
	if err != nil || len(requests) != 1 {
		t.Fatalf("requests=%v err=%v", requests, err)
	}
	r := requests[0]
	if r.RecipientID != "user-1" || r.Type != notification.TypeSavingsOpened || r.SourceEventID != "evt-o" || !r.OccurredAt.Equal(at) {
		t.Fatalf("wrong request: %+v", r)
	}
	title, body, data := render(t, r)
	if title != "Gửi tiết kiệm thành công" || body != "-10.000.000 VND từ tài khoản •••• 0111 gửi tiết kiệm kỳ hạn 6 tháng." ||
		data["savingsId"] != "sav-1" || data["amount"] != "10000000" || data["termMonths"] != "6" {
		t.Fatalf("title=%q body=%q data=%v", title, body, data)
	}
	if strings.Contains(body, "1000000111") {
		t.Fatal("full account number leaked")
	}
}

func TestSettledNotifiesOwnerWithInterest(t *testing.T) {
	requests, err := mapper().Map(context.Background(), settled())
	if err != nil || len(requests) != 1 || requests[0].Type != notification.TypeSavingsMatured {
		t.Fatalf("requests=%v err=%v", requests, err)
	}
	title, body, data := render(t, requests[0])
	if title != "Tất toán tiết kiệm thành công" || body != "+10.250.000 VND (gồm lãi 250.000 VND) vào tài khoản •••• 0111." || data["amount"] != "10250000" {
		t.Fatalf("title=%q body=%q data=%v", title, body, data)
	}
}

func TestIncompleteRecordsAreSkipped(t *testing.T) {
	for _, r := range []journal.Record{
		{ID: "e", Type: journal.SavingsOpened, At: at},
		{ID: "e", Type: journal.SavingsSettled, At: at, Savings: &journal.SavingsMovement{}},
	} {
		if requests, err := mapper().Map(context.Background(), r); err != nil || requests != nil {
			t.Fatalf("requests=%v err=%v", requests, err)
		}
	}
}

func TestMissingAccountAndLookupErrorsRetry(t *testing.T) {
	if _, err := NewMapper(fakeAccounts{}).Map(context.Background(), opened()); !errors.Is(err, errAccountNotReady) {
		t.Fatalf("missing account: %v", err)
	}
	if _, err := NewMapper(fakeAccounts{err: errors.New("mongo down")}).Map(context.Background(), opened()); err == nil {
		t.Fatal("lookup error must propagate")
	}
}

func TestOpenAndSettleNotificationsDoNotCollide(t *testing.T) {
	a, _ := mapper().Map(context.Background(), opened())
	b, _ := mapper().Map(context.Background(), settled())
	if notification.NotificationID(a[0].SourceEventID, a[0].Type, a[0].RecipientID) == notification.NotificationID(b[0].SourceEventID, b[0].Type, b[0].RecipientID) {
		t.Fatal("open and settle must be distinct notifications")
	}
}
