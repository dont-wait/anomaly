package reminder

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/timeutil"
)

// Installment is read-only data from MongoDB loans.repayment_schedule.
// DueDate is a date-only UTC value produced by the signed EMI schedule.
type Installment struct {
	LoanID        string
	LoanNo        string
	LoanStatus    string
	InstallmentNo int
	DueDate       time.Time
	TotalDue      int64
	Status        string
}

type ReminderType string

const (
	ReminderFiveDays ReminderType = "T-5"
	ReminderDueToday ReminderType = "T-0"
)

// Candidate identifies a reminder with a stable idempotency key. In stage 2,
// the shared Notification service persists a deterministic notification ID
// using MongoDB $setOnInsert. The in-process seen map is only a scan-level filter.
type Candidate struct {
	LoanID        string
	LoanNo        string
	InstallmentNo int
	DueDate       time.Time
	TotalDue      int64
	Type          ReminderType
}

func (c Candidate) IdempotencyKey() string {
	return fmt.Sprintf("loan-reminder:%s:%d:%s", c.LoanID, c.InstallmentNo, c.Type)
}

// Repository is supplied by MongoDB; it does not read EventStoreDB.
type Repository interface {
	FindUpcoming(ctx context.Context, datesUTC []time.Time) ([]Installment, error)
}

// Handler is a dry-run logger unless notifications are explicitly enabled.
type Handler interface {
	Handle(ctx context.Context, item Candidate) error
}

type HandlerFunc func(context.Context, Candidate) error

func (fn HandlerFunc) Handle(ctx context.Context, item Candidate) error {
	return fn(ctx, item)
}

// HandlerError identifies the failed reminder type without exposing the loan
// number, installment amount, or an untrusted underlying error in routine logs.
// errors.Is/errors.As still allow internal callers to inspect the cause.
type HandlerError struct {
	Type  ReminderType
	Cause error
}

func (e *HandlerError) Error() string {
	return fmt.Sprintf("loan reminder handler failed (type=%s)", e.Type)
}

func (e *HandlerError) Unwrap() error { return e.Cause }

type Stats struct {
	Scanned int
	Matched int
	Skipped int
	Failed  int
}

type Job struct {
	Repository Repository
	Handler    Handler
}

// TargetDates returns UTC calendar dates because the stored EMI dates are
// UTC-midnight date-only values. 'Today' is determined in Vietnam time.
func TargetDates(now time.Time) (today, inFiveDays time.Time) {
	y, m, d := now.In(timeutil.VietnamLocation).Date()
	today = time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return today, today.AddDate(0, 0, 5)
}

func (j Job) Run(ctx context.Context, now time.Time) (Stats, error) {
	if j.Repository == nil || j.Handler == nil {
		return Stats{}, errors.New("loan reminder repository and handler are required")
	}
	if err := ctx.Err(); err != nil {
		return Stats{}, err
	}

	today, inFiveDays := TargetDates(now)
	rows, err := j.Repository.FindUpcoming(ctx, []time.Time{today, inFiveDays})
	if err != nil {
		return Stats{}, fmt.Errorf("find upcoming loan repayments: %w", err)
	}

	stats := Stats{Scanned: len(rows)}
	seen := make(map[string]struct{}, len(rows))
	var handlerErrors []error
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		if row.LoanStatus != "active" || row.Status != "upcoming" ||
			strings.TrimSpace(row.LoanID) == "" || row.InstallmentNo <= 0 ||
			row.TotalDue <= 0 || row.DueDate.IsZero() {
			stats.Skipped++
			continue
		}

		// The projection stores UTC dates; do not convert the timestamp to a
		// local wall-clock time before comparing its calendar day.
		date := row.DueDate.UTC().Format("2006-01-02")
		kind := ReminderType("")
		switch date {
		case today.Format("2006-01-02"):
			kind = ReminderDueToday
		case inFiveDays.Format("2006-01-02"):
			kind = ReminderFiveDays
		default:
			stats.Skipped++
			continue
		}

		item := Candidate{
			LoanID:        row.LoanID,
			LoanNo:        row.LoanNo,
			InstallmentNo: row.InstallmentNo,
			DueDate:       row.DueDate,
			TotalDue:      row.TotalDue,
			Type:          kind,
		}
		key := item.IdempotencyKey()
		if _, ok := seen[key]; ok {
			stats.Skipped++
			continue
		}
		seen[key] = struct{}{}
		stats.Matched++
		if err := j.Handler.Handle(ctx, item); err != nil {
			if errors.Is(err, ErrNoLongerEligible) {
				stats.Skipped++
				continue
			}
			stats.Failed++
			handlerErrors = append(handlerErrors, &HandlerError{Type: kind, Cause: err})
			// Continue to other candidates; surface all errors afterwards.
		}
	}
	return stats, errors.Join(handlerErrors...)
}
