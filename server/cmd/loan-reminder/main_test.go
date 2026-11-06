package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/application/loan/reminder"
	"github.com/dont-wait/anomaly/internal/scheduler"
	"github.com/rs/zerolog"
)

type blockingReminderRepository struct{}

func (blockingReminderRepository) FindUpcoming(ctx context.Context, _, _ time.Time) ([]reminder.Installment, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type failedReminderRepository struct{}

func (failedReminderRepository) FindUpcoming(context.Context, time.Time, time.Time) ([]reminder.Installment, error) {
	return nil, errors.New("query unavailable")
}

type countingReminderRepository struct {
	calls int
	fail  bool
	rows  []reminder.Installment
}

func (r *countingReminderRepository) FindUpcoming(context.Context, time.Time, time.Time) ([]reminder.Installment, error) {
	r.calls++
	if r.fail {
		return nil, errors.New("scan failed")
	}
	return r.rows, nil
}

func TestDryRunLogContainsNoFinancialIdentifiers(t *testing.T) {
	var output bytes.Buffer
	log := zerolog.New(&output)
	item := reminder.Candidate{
		LoanID:        "PRIVATE-LOAN-ID",
		LoanNo:        "PRIVATE-LOAN-NO",
		InstallmentNo: 3,
		DueDate:       time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC),
		TotalDue:      123456789,
		Type:          reminder.ReminderFiveDays,
	}
	logDryRunCandidate(&log, item)
	got := output.String()
	for _, forbidden := range []string{"PRIVATE-LOAN-ID", "PRIVATE-LOAN-NO", "123456789", "2026-11-09", "idempotency_key", "due_date", "installment_no"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("dry-run log leaked %q: %s", forbidden, got)
		}
	}
	if !strings.Contains(got, `"type":"T-5"`) {
		t.Fatalf("missing non-sensitive reminder type: %s", got)
	}
}

func TestProductionRunnerPropagatesRepositoryError(t *testing.T) {
	var output bytes.Buffer
	log := zerolog.New(&output)
	job := reminder.Job{Repository: failedReminderRepository{}, Handler: reminder.HandlerFunc(func(context.Context, reminder.Candidate) error { return nil })}
	runner, err := newReminderRunner(&log, job, "dry-run", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	got := runner.RunOnce(context.Background())
	if got == nil || !strings.Contains(got.Error(), "query unavailable") {
		t.Fatalf("expected repository error, got %v", got)
	}
	if strings.Contains(output.String(), "scan completed") || !strings.Contains(output.String(), "scan failed") {
		t.Fatalf("failed scan reported as success: %s", output.String())
	}
}

func TestProductionRunnerCancelsSlowQuery(t *testing.T) {
	log := zerolog.Nop()
	job := reminder.Job{Repository: blockingReminderRepository{}, Handler: reminder.HandlerFunc(func(context.Context, reminder.Candidate) error { return nil })}
	runner, err := newReminderRunner(&log, job, "dry-run", 15*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if got := runner.RunOnce(context.Background()); !errors.Is(got, context.DeadlineExceeded) {
		t.Fatalf("expected scan timeout, got %v", got)
	}
}

func TestNotificationScansOnlyInVietnamDaytimeSlots(t *testing.T) {
	var output bytes.Buffer
	log := zerolog.New(&output)
	repo := &countingReminderRepository{}
	job := reminder.Job{Repository: repo, Handler: reminder.HandlerFunc(func(context.Context, reminder.Candidate) error { return nil })}
	// A fixed clock flows into the exact closure that production registers.
	now := time.Date(2026, 11, 6, 1, 0, 0, 0, time.UTC) // 08:00 Vietnam
	runner, err := newReminderRunner(&log, job, "notification", time.Second, scheduler.WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	check := func(hour int, want int) {
		t.Helper()
		now = time.Date(2026, 11, 6, hour-7, 0, 0, 0, time.UTC)
		if err := runner.RunOnce(context.Background()); err != nil {
			t.Fatalf("hour %d: %v", hour, err)
		}
		if repo.calls != want {
			t.Fatalf("hour %d: calls=%d, want=%d", hour, repo.calls, want)
		}
	}
	check(8, 0)
	check(9, 1)
	check(10, 1)
	check(15, 1)
	check(16, 2)
	check(20, 2)
	check(21, 2)
	// A new Vietnam calendar day always gets a new morning scan.
	now = time.Date(2026, 11, 7, 2, 0, 0, 0, time.UTC)
	if err := runner.RunOnce(context.Background()); err != nil || repo.calls != 3 {
		t.Fatalf("next day: calls=%d err=%v", repo.calls, err)
	}
}

func TestProductionRunnerRetriesFailedNotificationSlot(t *testing.T) {
	log := zerolog.Nop()
	repo := &countingReminderRepository{fail: true}
	job := reminder.Job{Repository: repo, Handler: reminder.HandlerFunc(func(context.Context, reminder.Candidate) error { return nil })}
	now := time.Date(2026, 11, 6, 3, 0, 0, 0, time.UTC) // 10:00 Vietnam
	runner, err := newReminderRunner(&log, job, "notification", time.Second, scheduler.WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.RunOnce(context.Background()); err == nil || repo.calls != 1 {
		t.Fatalf("expected first failure, calls=%d err=%v", repo.calls, err)
	}
	repo.fail = false
	if err := runner.RunOnce(context.Background()); err != nil || repo.calls != 2 {
		t.Fatalf("expected retry, calls=%d err=%v", repo.calls, err)
	}
	if err := runner.RunOnce(context.Background()); err != nil || repo.calls != 2 {
		t.Fatalf("success should not immediately repeat, calls=%d err=%v", repo.calls, err)
	}
}
