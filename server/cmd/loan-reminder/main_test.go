package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/application/loan/reminder"
	"github.com/rs/zerolog"
)

type blockingReminderRepository struct{}

func (blockingReminderRepository) FindUpcoming(ctx context.Context, _ []time.Time) ([]reminder.Installment, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type failedReminderRepository struct{}

func (failedReminderRepository) FindUpcoming(context.Context, []time.Time) ([]reminder.Installment, error) {
	return nil, errors.New("query unavailable")
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

func TestRunScanPropagatesRepositoryError(t *testing.T) {
	job := reminder.Job{
		Repository: failedReminderRepository{},
		Handler:    reminder.HandlerFunc(func(context.Context, reminder.Candidate) error { return nil }),
	}
	_, err := runScan(context.Background(), job, time.Now(), time.Second)
	if err == nil || !strings.Contains(err.Error(), "query unavailable") {
		t.Fatalf("expected repository error, got %v", err)
	}
}

func TestRunScanCancelsSlowQuery(t *testing.T) {
	job := reminder.Job{
		Repository: blockingReminderRepository{},
		Handler:    reminder.HandlerFunc(func(context.Context, reminder.Candidate) error { return nil }),
	}
	_, err := runScan(context.Background(), job, time.Now(), 15*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected scan timeout, got %v", err)
	}
}
