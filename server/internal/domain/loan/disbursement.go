package loan

import (
	"fmt"
	"time"
)

func (a *Application) MarkDisbursed(
	at time.Time,
	annualRateBps int64,
) error {
	if a == nil ||
		a.Status != ApplicationStatusApproved ||
		a.ApprovedAt == nil {
		return ErrInvalidApplicationStatus
	}

	if at.IsZero() ||
		at.Before(*a.ApprovedAt) ||
		annualRateBps <= 0 {
		return fmt.Errorf("invalid loan disbursement terms")
	}

	schedule, err := CalculateRepaymentSchedule(
		a.RequestedAmount,
		annualRateBps,
		a.TermMonths,
		at,
	)
	if err != nil {
		return err
	}

	a.Status = ApplicationStatusDisbursed
	a.DisbursedAt = &at
	a.AnnualRateBps = annualRateBps
	a.RepaymentSchedule = schedule
	a.UpdatedAt = at

	return nil
}
