package reminder

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	notificationapp "github.com/dont-wait/anomaly/internal/application/notification"
	"github.com/dont-wait/anomaly/internal/domain/notification"
)

type fakeRecipientLookup struct {
	recipient string
	eligible  bool
	err       error
	calls     int
}

func (f *fakeRecipientLookup) ResolveEligible(_ context.Context, _ Candidate) (string, bool, error) {
	f.calls++
	return f.recipient, f.eligible, f.err
}

type fakeNotifier struct {
	requests []notificationapp.Request
	err      error
}

func (f *fakeNotifier) Notify(_ context.Context, items ...notificationapp.Request) error {
	f.requests = append(f.requests, items...)
	return f.err
}

func candidateForTest(now time.Time, kind ReminderType) Candidate {
	today, five := TargetDates(now)
	due := today
	if kind == ReminderFiveDays {
		due = five
	}
	return Candidate{LoanID: "loan-uuid", LoanNo: "PRIVATE-LOAN", InstallmentNo: 2, DueDate: due, TotalDue: 1500000, Type: kind, ScanAt: now}
}

func TestNotificationHandlerT5AndT0UseDifferentDurableSourceIDs(t *testing.T) {
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, time.UTC)
	lookup := &fakeRecipientLookup{recipient: "JWT-user-id", eligible: true}
	notifier := &fakeNotifier{}
	handler := NotificationHandler{Lookup: lookup, Notifier: notifier}
	for _, kind := range []ReminderType{ReminderFiveDays, ReminderDueToday} {
		if err := handler.Handle(context.Background(), candidateForTest(now, kind)); err != nil {
			t.Fatal(err)
		}
	}
	if lookup.calls != 2 || len(notifier.requests) != 2 {
		t.Fatalf("lookup=%d notified=%d", lookup.calls, len(notifier.requests))
	}
	a, b := notifier.requests[0], notifier.requests[1]
	if a.Audience != notification.AudienceCustomer || a.RecipientID != "JWT-user-id" || a.Type != notification.TypeLoanRepaymentDue || !a.OccurredAt.Equal(now) {
		t.Fatalf("wrong request: %+v", a)
	}
	if a.SourceEventID == b.SourceEventID || a.SourceEventID != "loan-reminder:loan-uuid:2:T-5" || b.SourceEventID != "loan-reminder:loan-uuid:2:T-0" {
		t.Fatalf("incorrect deterministic IDs: %s / %s", a.SourceEventID, b.SourceEventID)
	}
	if a.Params["loanId"] != "loan-uuid" || a.Params["kind"] != "T-5" || b.Params["kind"] != "T-0" {
		t.Fatalf("bad template params: %+v %+v", a.Params, b.Params)
	}
}

func TestNotificationHandlerSkipsPaidOrOutOfWindow(t *testing.T) {
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, time.UTC)
	lookup := &fakeRecipientLookup{recipient: "JWT-user-id", eligible: false}
	notifier := &fakeNotifier{}
	handler := NotificationHandler{Lookup: lookup, Notifier: notifier}
	if err := handler.Handle(context.Background(), candidateForTest(now, ReminderDueToday)); !errors.Is(err, ErrNoLongerEligible) {
		t.Fatalf("paid installment should be skipped: %v", err)
	}
	stale := candidateForTest(now, ReminderFiveDays)
	stale.DueDate = stale.DueDate.AddDate(0, 0, 1)
	if err := handler.Handle(context.Background(), stale); !errors.Is(err, ErrNoLongerEligible) {
		t.Fatalf("wrong calendar day should be skipped: %v", err)
	}
	if lookup.calls != 1 || len(notifier.requests) != 0 {
		t.Fatalf("unexpected calls %d requests %d", lookup.calls, len(notifier.requests))
	}
}

func TestNotificationHandlerRetriesUnavailableOwnerAndDeliveryErrors(t *testing.T) {
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, time.UTC)
	lookup := &fakeRecipientLookup{eligible: true}
	notifier := &fakeNotifier{}
	handler := NotificationHandler{Lookup: lookup, Notifier: notifier}
	item := candidateForTest(now, ReminderDueToday)
	if err := handler.Handle(context.Background(), item); err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("missing owner must be an error: %v", err)
	}
	failure := errors.New("storage unavailable")
	lookup.recipient = "JWT-user-id"
	notifier.err = failure
	if err := handler.Handle(context.Background(), item); !errors.Is(err, failure) {
		t.Fatalf("notifier failure not propagated: %v", err)
	}
	lookup.err = errors.New("ownership projection unavailable")
	if err := handler.Handle(context.Background(), item); err == nil {
		t.Fatal("lookup error not propagated")
	}
}

func TestHandlerDoesNotUseWallClockAfterMidnight(t *testing.T) {
	// ScanAt was chosen before midnight; the handler must not call time.Now.
	now := time.Date(2026, 11, 5, 16, 59, 59, 0, time.UTC)
	lookup := &fakeRecipientLookup{recipient: "JWT-user-id", eligible: true}
	notifier := &fakeNotifier{}
	handler := NotificationHandler{Lookup: lookup, Notifier: notifier}
	item := candidateForTest(now, ReminderDueToday)
	if err := handler.Handle(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if len(notifier.requests) != 1 || !notifier.requests[0].OccurredAt.Equal(now) {
		t.Fatalf("handler ignored scan timestamp: %+v", notifier.requests)
	}
}
