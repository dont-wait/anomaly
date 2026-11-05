package reminder

import (
	"context"
	"errors"
	"fmt"
	"strings"

	notificationapp "github.com/dont-wait/anomaly/internal/application/notification"
	"github.com/dont-wait/anomaly/internal/domain/notification"
)

// ErrNoLongerEligible indicates a paid, cancelled, or out-of-window reminder.
// It is an expected skip, not a delivery failure.
var ErrNoLongerEligible = errors.New("loan reminder is no longer eligible")

// RecipientLookup rechecks the live loan projection and resolves the JWT user
// ID. The owner must come from loan_applications_read.owner_id, NOT the
// financial customer_id in accounts or loan_applications.
type RecipientLookup interface {
	ResolveEligible(ctx context.Context, candidate Candidate) (recipientID string, eligible bool, err error)
}

// NotificationHandler writes through the existing Notification service, which
// handles deterministic IDs and MongoDB $setOnInsert idempotency. This creates
// in-app notifications; it is NOT an SMTP/email sender.
type NotificationHandler struct {
	Lookup   RecipientLookup
	Notifier notificationapp.Notifier
}

func (h NotificationHandler) Handle(ctx context.Context, item Candidate) error {
	if h.Lookup == nil || h.Notifier == nil {
		return errors.New("loan reminder notification handler is not configured")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// ScanAt is fixed at scheduler dispatch, so midnight cannot invalidate
	// otherwise eligible work or make WithNow-based tests nondeterministic.
	if !EligibleForKind(item.ScanAt, item.DueDate, item.Type) {
		return ErrNoLongerEligible
	}

	if strings.TrimSpace(item.LoanID) == "" || item.InstallmentNo <= 0 || item.TotalDue <= 0 {
		return errors.New("invalid loan reminder candidate")
	}

	recipientID, eligible, err := h.Lookup.ResolveEligible(ctx, item)
	if err != nil {
		return fmt.Errorf("recheck loan reminder eligibility: %w", err)
	}
	if !eligible {
		return ErrNoLongerEligible
	}
	if strings.TrimSpace(recipientID) == "" {
		return errors.New("loan reminder owner projection is missing")
	}
	return h.Notifier.Notify(ctx, notificationapp.Request{
		Audience:    notification.AudienceCustomer,
		RecipientID: recipientID,
		Type:        notification.TypeLoanRepaymentDue,
		// Type T-5 or T-0 is embedded in the stable source identifier, so the
		// same installment can receive exactly one notification at each stage.
		SourceEventID: item.IdempotencyKey(),
		OccurredAt:    item.ScanAt.UTC(),
		Params: map[string]string{
			"kind":   string(item.Type),
			"loanId": item.LoanID,
		},
	})
}
