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
	start time.Time
	end   time.Time
	err   error
}

func (f *fakeRepository) FindUpcoming(_ context.Context, start, end time.Time) ([]Installment, error) {
	f.start, f.end = start, end
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
		row("d", 1, today.AddDate(0, 0, 6), "upcoming"),
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
	if !repo.start.Equal(today.AddDate(0, 0, -DueCatchUpDays)) || !repo.end.Equal(five.AddDate(0, 0, 1)) {
		t.Fatalf("query window: %s..%s", repo.start, repo.end)
	}
	if stats != (Stats{Scanned: 6, Matched: 2, Skipped: 4}) {
		t.Fatalf("stats = %+v", stats)
	}
	if len(candidates) != 2 || candidates[0].Type != ReminderFiveDays || candidates[1].Type != ReminderDueToday || !candidates[0].ScanAt.Equal(now) {
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

func TestKindForDateCatchUpWindows(t *testing.T) {
	now := time.Date(2026, 11, 6, 2, 0, 0, 0, time.UTC) // 09:00 Vietnam
	today, _ := TargetDates(now)
	cases := []struct {
		offset int
		kind   ReminderType
	}{
		{-4, ""},
		{-3, ReminderDueToday},
		{-2, ReminderDueToday},
		{-1, ReminderDueToday},
		{0, ReminderDueToday},
		{1, ReminderFiveDays},
		{2, ReminderFiveDays},
		{3, ReminderFiveDays},
		{4, ReminderFiveDays},
		{5, ReminderFiveDays},
		{6, ""},
	}
	for _, tc := range cases {
		due := today.AddDate(0, 0, tc.offset)
		if got := KindForDate(now, due); got != tc.kind {
			t.Errorf("offset %d: got %q, want %q", tc.offset, got, tc.kind)
		}
	}
}

func TestJobUsesOneScanInstantAcrossMidnight(t *testing.T) {
	// A scan that starts before midnight Vietnam time must keep that date
	// even if handler work continues after midnight.
	now := time.Date(2026, 11, 5, 16, 59, 59, 0, time.UTC)
	today, _ := TargetDates(now)
	repo := &fakeRepository{rows: []Installment{{
		LoanID: "a", LoanStatus: "active", InstallmentNo: 1,
		DueDate: today, TotalDue: 100, Status: "upcoming",
	}}}
	var received time.Time
	job := Job{Repository: repo, Handler: HandlerFunc(func(_ context.Context, item Candidate) error {
		received = item.ScanAt
		return nil
	})}
	stats, err := job.Run(context.Background(), now)
	if err != nil || stats.Matched != 1 || !received.Equal(now) {
		t.Fatalf("wrong scan snapshot: stats=%+v now=%v err=%v", stats, received, err)
	}
}

func TestJobCatchesUpT5OnFollowingDays(t *testing.T) {
	now := time.Date(2026, 11, 6, 4, 0, 0, 0, time.UTC)
	today, _ := TargetDates(now)
	rows := make([]Installment, 0, 5)
	for i := 1; i <= 5; i++ {
		rows = append(rows, Installment{
			LoanID: "loan", LoanStatus: "active", InstallmentNo: i,
			DueDate: today.AddDate(0, 0, i), TotalDue: 100, Status: "upcoming",
		})
	}
	repo := &fakeRepository{rows: rows}
	var got []Candidate
	job := Job{Repository: repo, Handler: HandlerFunc(func(_ context.Context, item Candidate) error {
		got = append(got, item)
		return nil
	})}
	stats, err := job.Run(context.Background(), now)
	if err != nil || stats.Matched != 5 || len(got) != 5 {
		t.Fatalf("catch-up failed: %+v %v (%d candidates)", stats, err, len(got))
	}
	for _, item := range got {
		if item.Type != ReminderFiveDays || !item.ScanAt.Equal(now) {
			t.Fatalf("wrong catch-up identity or scan instant: %+v", item)
		}
	}
}

func TestJobCatchesUpDueDayWhenWorkerWasOffline(t *testing.T) {
	now := time.Date(2026, 11, 6, 3, 0, 0, 0, time.UTC) // Nov 6, 10am Vietnam
	today, _ := TargetDates(now)
	repo := &fakeRepository{rows: []Installment{
		{LoanID: "a", LoanStatus: "active", InstallmentNo: 1, DueDate: today.AddDate(0, 0, -1), TotalDue: 100, Status: "overdue"},
		{LoanID: "b", LoanStatus: "active", InstallmentNo: 1, DueDate: today.AddDate(0, 0, -2), TotalDue: 100, Status: "upcoming"},
		{LoanID: "c", LoanStatus: "active", InstallmentNo: 1, DueDate: today.AddDate(0, 0, -1), TotalDue: 100, Status: "paid"},
		{LoanID: "d", LoanStatus: "active", InstallmentNo: 1, DueDate: today.AddDate(0, 0, 1), TotalDue: 100, Status: "overdue"},
	}}
	var candidates []Candidate
	job := Job{Repository: repo, Handler: HandlerFunc(func(_ context.Context, item Candidate) error {
		candidates = append(candidates, item)
		return nil
	})}
	stats, err := job.Run(context.Background(), now)
	if err != nil || stats.Matched != 2 || stats.Skipped != 2 {
		t.Fatalf("overdue recovery: %+v %v", stats, err)
	}
	for _, candidate := range candidates {
		if candidate.Type != ReminderDueToday {
			t.Fatalf("overdue reminder must reuse T-0 stable key: %+v", candidate)
		}
	}
}
