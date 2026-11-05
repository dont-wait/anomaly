package reminder

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeRepository struct {
	rows  []Installment
	dates []time.Time
	err   error
}

func (f *fakeRepository) FindUpcoming(_ context.Context, dates []time.Time) ([]Installment, error) {
	f.dates = append([]time.Time(nil), dates...)
	return f.rows, f.err
}

func TestTargetDatesUsesVietnamDayAndUTCCalendarDate(t *testing.T) {
	// 18:30 UTC is already the next calendar day in Vietnam.
	today, five := TargetDates(time.Date(2026, 11, 4, 18, 30, 0, 0, time.UTC))
	if got := today.Format(time.RFC3339); got != "2026-11-05T00:00:00Z" {
		t.Fatalf("today = %s", got)
	}
	if got := five.Format("2006-01-02"); got != "2026-11-10" {
		t.Fatalf("T-5 = %s", got)
	}
	// Crossing month/year boundaries must still add five calendar days.
	_, five = TargetDates(time.Date(2026, 12, 29, 12, 0, 0, 0, time.UTC))
	if got := five.Format("2006-01-02"); got != "2027-01-03" {
		t.Fatalf("year boundary = %s", got)
	}
}

func TestJobFiltersDueDatesStatusesAndDuplicates(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	today, five := TargetDates(now)
	row := func(loan string, installment int, due time.Time, status string) Installment {
		return Installment{LoanID: loan, LoanNo: "LN-" + loan, LoanStatus: "active", InstallmentNo: installment, DueDate: due, TotalDue: 2500000, Status: status}
	}
	repo := &fakeRepository{rows: []Installment{
		row("a", 1, five, "upcoming"),
		row("b", 2, today, "upcoming"),
		row("b", 2, today, "upcoming"), // duplicate projection row
		row("c", 1, today, "paid"),
		row("d", 1, today.AddDate(0, 0, 1), "upcoming"),
		row("e", 1, today, "upcoming"),
	}}
	repo.rows[5].LoanStatus = "completed"
	var candidates []Candidate
	job := Job{Repository: repo, Handler: HandlerFunc(func(_ context.Context, item Candidate) error {
		candidates = append(candidates, item)
		return nil
	})}
	stats, err := job.Run(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.dates) != 2 || !repo.dates[0].Equal(today) || !repo.dates[1].Equal(five) {
		t.Fatalf("query dates = %v", repo.dates)
	}
	if stats != (Stats{Scanned: 6, Matched: 2, Skipped: 4}) {
		t.Fatalf("stats = %+v", stats)
	}
	if len(candidates) != 2 || candidates[0].Type != ReminderFiveDays || candidates[1].Type != ReminderDueToday {
		t.Fatalf("candidates = %+v", candidates)
	}
	if candidates[0].IdempotencyKey() != "loan-reminder:a:1:T-5" || candidates[1].IdempotencyKey() != "loan-reminder:b:2:T-0" {
		t.Fatalf("unexpected keys: %s %s", candidates[0].IdempotencyKey(), candidates[1].IdempotencyKey())
	}
}

func TestJobContinuesAfterCandidateHandlerFailure(t *testing.T) {
	today, _ := TargetDates(time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC))
	repo := &fakeRepository{rows: []Installment{
		{LoanID: "a", LoanStatus: "active", InstallmentNo: 1, DueDate: today, TotalDue: 100, Status: "upcoming"},
		{LoanID: "b", LoanStatus: "active", InstallmentNo: 1, DueDate: today, TotalDue: 200, Status: "upcoming"},
	}}
	calls := 0
	failure := errors.New("PRIVATE-LOAN-ID: temporary delivery failure")
	job := Job{Repository: repo, Handler: HandlerFunc(func(_ context.Context, _ Candidate) error {
		calls++
		if calls == 1 {
			return failure
		}
		return nil
	})}
	stats, err := job.Run(context.Background(), time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC))
	if calls != 2 || stats.Matched != 2 || stats.Failed != 1 || !errors.Is(err, failure) {
		t.Fatalf("calls=%d stats=%+v err=%v", calls, stats, err)
	}
	var handlerError *HandlerError
	if !errors.As(err, &handlerError) || handlerError.Type != ReminderDueToday {
		t.Fatalf("expected typed handler error: %v", err)
	}
	if strings.Contains(err.Error(), "PRIVATE-LOAN-ID") || !strings.Contains(err.Error(), "T-0") {
		t.Fatalf("error summary leaked sensitive data or omitted type: %s", err)
	}
}

func TestJobJoinsMultipleHandlerFailures(t *testing.T) {
	today, five := TargetDates(time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC))
	repo := &fakeRepository{rows: []Installment{
		{LoanID: "a", LoanStatus: "active", InstallmentNo: 1, DueDate: today, TotalDue: 100, Status: "upcoming"},
		{LoanID: "b", LoanStatus: "active", InstallmentNo: 1, DueDate: five, TotalDue: 200, Status: "upcoming"},
	}}
	first, second := errors.New("private first error"), errors.New("private second error")
	calls := 0
	job := Job{Repository: repo, Handler: HandlerFunc(func(_ context.Context, _ Candidate) error {
		calls++
		if calls == 1 {
			return first
		}
		return second
	})}
	stats, err := job.Run(context.Background(), time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC))
	if calls != 2 || stats.Failed != 2 || !errors.Is(err, first) || !errors.Is(err, second) {
		t.Fatalf("calls=%d stats=%+v err=%v", calls, stats, err)
	}
	if strings.Contains(err.Error(), "private") || !strings.Contains(err.Error(), "T-0") || !strings.Contains(err.Error(), "T-5") {
		t.Fatalf("unsafe aggregated error summary: %s", err)
	}
}

func TestJobPropagatesRepositoryError(t *testing.T) {
	job := Job{Repository: &fakeRepository{err: errors.New("mongodb unavailable")}, Handler: HandlerFunc(func(context.Context, Candidate) error { return nil })}
	if _, err := job.Run(context.Background(), time.Now()); err == nil {
		t.Fatal("expected repository error")
	}
}

func TestJobTreatsPaidBetweenScanAndDeliveryAsSkip(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	today, _ := TargetDates(now)
	repo := &fakeRepository{rows: []Installment{{LoanID: "loan-a", LoanStatus: "active", InstallmentNo: 1, DueDate: today, TotalDue: 100, Status: "upcoming"}}}
	job := Job{Repository: repo, Handler: HandlerFunc(func(context.Context, Candidate) error { return ErrNoLongerEligible })}
	stats, err := job.Run(context.Background(), now)
	if err != nil || stats.Matched != 1 || stats.Skipped != 1 || stats.Failed != 0 {
		t.Fatalf("stale paid installment should be skipped: stats=%+v err=%v", stats, err)
	}
}
