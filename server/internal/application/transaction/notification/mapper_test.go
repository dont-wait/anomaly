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
	byFinancial map[string]*accountdomain.UserAccount
	err         error
}

func (f fakeAccounts) FindByFinancialID(_ context.Context, id string) (*accountdomain.UserAccount, error) {
	return f.byFinancial[id], f.err
}

type fakeCustomers map[string]*accountdomain.Customer

func (f fakeCustomers) FindByID(_ context.Context, id string) (*accountdomain.Customer, error) {
	return f[id], nil
}

func fixture() (*Mapper, journal.Record) {
	accounts := fakeAccounts{byFinancial: map[string]*accountdomain.UserAccount{
		"fin-src": {Id: "user-src", AccountNo: "1000000111", CustomerId: "cus-src"},
		"fin-dst": {Id: "user-dst", AccountNo: "2000000222", CustomerId: "cus-dst"},
	}}
	customers := fakeCustomers{
		"cus-src": {Profile: accountdomain.CustomerProfile{FullName: "Nguyen Van A"}},
		"cus-dst": {Profile: accountdomain.CustomerProfile{FullName: "Tran Thi B"}},
	}
	record := journal.Record{
		ID: "evt-1", Type: journal.Completed, At: time.Date(2026, 11, 7, 3, 0, 0, 0, time.UTC),
		Event: &journal.Event{TransactionID: "tx-1", Payload: journal.Payload{
			SourceAccountID: "fin-src", DestinationAccountID: "fin-dst", Amount: 1500000, Currency: "VND",
		}},
	}
	return NewMapper(accounts, customers), record
}

func TestSupportsOnlyTransferCompleted(t *testing.T) {
	m, _ := fixture()
	if !m.Supports(journal.Completed) || m.Supports(journal.Failed) || m.Supports(journal.Created) {
		t.Fatal("mapper must only support TransferCompleted")
	}
}

func TestMapNotifiesBothParties(t *testing.T) {
	m, record := fixture()
	requests, err := m.Map(context.Background(), record)
	if err != nil || len(requests) != 2 {
		t.Fatalf("requests=%d err=%v", len(requests), err)
	}
	sent, received := requests[0], requests[1]
	if sent.RecipientID != "user-src" || sent.Type != notification.TypeTransferSent ||
		received.RecipientID != "user-dst" || received.Type != notification.TypeTransferReceived {
		t.Fatalf("wrong recipients/types: %+v %+v", sent, received)
	}
	for _, r := range requests {
		if r.SourceEventID != "evt-1" || !r.OccurredAt.Equal(record.At) || r.Audience != notification.AudienceCustomer {
			t.Fatalf("wrong metadata: %+v", r)
		}
	}
	// The payer sees the payee and vice versa.
	if sent.Params["counterpartyName"] != "Tran Thi B" || sent.Params["last4"] != "0222" ||
		received.Params["counterpartyName"] != "Nguyen Van A" || received.Params["last4"] != "0111" {
		t.Fatalf("wrong counterparty: %v %v", sent.Params, received.Params)
	}
	if sent.Params["amount"] != "1.500.000" || sent.Params["transferId"] != "tx-1" {
		t.Fatalf("wrong params: %v", sent.Params)
	}
}

func TestMapRequestsRenderThroughRegisteredTemplates(t *testing.T) {
	m, record := fixture()
	requests, err := m.Map(context.Background(), record)
	if err != nil || len(requests) != 2 {
		t.Fatalf("requests=%d err=%v", len(requests), err)
	}
	want := []string{"-1.500.000 VND tới Tran Thi B (•••• 0222).", "+1.500.000 VND từ Nguyen Van A (•••• 0111)."}
	for i, r := range requests {
		tmpl, ok := notificationapp.Lookup(r.Type)
		if !ok {
			t.Fatalf("no template for %s", r.Type)
		}
		_, body, data, err := tmpl.Render(r.Params)
		if err != nil || body != want[i] || data["transferId"] != "tx-1" || data["amount"] != "1500000" || data["currency"] != "VND" {
			t.Fatalf("body=%q data=%v err=%v", body, data, err)
		}
		if strings.Contains(body, "1000000111") || strings.Contains(body, "2000000222") {
			t.Fatal("full account number leaked")
		}
	}
}

func TestMapIsDeterministicForReplay(t *testing.T) {
	m, record := fixture()
	a, _ := m.Map(context.Background(), record)
	b, _ := m.Map(context.Background(), record)
	for i := range a {
		idA := notification.NotificationID(a[i].SourceEventID, a[i].Type, a[i].RecipientID)
		idB := notification.NotificationID(b[i].SourceEventID, b[i].Type, b[i].RecipientID)
		if idA != idB {
			t.Fatal("replay must converge on the same notification id")
		}
	}
	if notification.NotificationID("evt-1", a[0].Type, a[0].RecipientID) == notification.NotificationID("evt-1", a[1].Type, a[1].RecipientID) {
		t.Fatal("sender and receiver notifications must differ")
	}
}

func TestMapFallsBackWhenProfileHasNoName(t *testing.T) {
	m, record := fixture()
	m.customers = fakeCustomers{}
	requests, err := m.Map(context.Background(), record)
	if err != nil || requests[0].Params["counterpartyName"] != "tài khoản" {
		t.Fatalf("fallback: %v err=%v", requests, err)
	}
	tmpl, _ := notificationapp.Lookup(requests[0].Type)
	_, body, _, err := tmpl.Render(requests[0].Params)
	if err != nil || body != "-1.500.000 VND tới tài khoản (•••• 0222)." {
		t.Fatalf("body=%q err=%v", body, err)
	}
}

func TestMapErrorsWhenAccountMissingOrLookupFails(t *testing.T) {
	for _, missing := range []string{"fin-src", "fin-dst"} {
		m, record := fixture()
		delete(m.accounts.(fakeAccounts).byFinancial, missing)
		if _, err := m.Map(context.Background(), record); !errors.Is(err, errAccountNotReady) {
			t.Fatalf("missing %s: %v", missing, err)
		}
	}
	m, record := fixture()
	m.accounts = fakeAccounts{err: errors.New("mongo down")}
	if _, err := m.Map(context.Background(), record); err == nil {
		t.Fatal("lookup error must propagate for retry")
	}
}

func TestFormatVND(t *testing.T) {
	for in, want := range map[int64]string{0: "0", 999: "999", 1000: "1.000", 1500000: "1.500.000", 12345678901: "12.345.678.901"} {
		if got := formatVND(in); got != want {
			t.Fatalf("%d: got %s want %s", in, got, want)
		}
	}
}
