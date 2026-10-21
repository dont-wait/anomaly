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
	if a.Offer != nil && (a.Offer.Status != LoanOfferConfirmed || a.Offer.ConfirmedAt == nil || a.Offer.Amount != a.RequestedAmount || a.Offer.TermMonths != a.TermMonths || a.Offer.AnnualRateBps != annualRateBps) {
		return ErrLoanContractNotConfirmed
	}
	// Không cho phép thay đổi lãi suất đã chốt.
	if a.DisbursementPreparedAt != nil {
		if a.PreparedAnnualRateBps <= 0 ||
			annualRateBps != a.PreparedAnnualRateBps {
			return fmt.Errorf(
				"disbursement interest rate differs from prepared terms",
			)
		}
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
