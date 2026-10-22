package loan

import (
	"fmt"
	"time"
)

// MarkDisbursed finalizes a new loan and requires a confirmed offer.
func (a *Application) MarkDisbursed(at time.Time, annualRateBps int64) error {
	return a.markDisbursed(at, annualRateBps, false)
}

// RestoreLegacyDisbursement is reserved for replay of already committed old
// LoanDisbursed events and explicitly authorized reconciliation of pre-rollout
// bank credits. Never call it to issue a new credit.
func (a *Application) RestoreLegacyDisbursement(at time.Time, annualRateBps int64) error {
	return a.markDisbursed(at, annualRateBps, true)
}

func (a *Application) markDisbursed(at time.Time, annualRateBps int64, historicalCredit bool) error {
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
	if a.Offer == nil && !historicalCredit {
		return ErrLoanContractNotConfirmed
	}
	if a.Offer != nil && (a.Offer.Status != LoanOfferConfirmed || a.Offer.ConfirmedAt == nil || a.Offer.Amount != a.RequestedAmount || a.Offer.TermMonths != a.TermMonths || a.Offer.AnnualRateBps != annualRateBps || at.Before(*a.Offer.ConfirmedAt)) {
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
