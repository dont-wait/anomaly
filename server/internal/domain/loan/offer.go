package loan

import (
	"errors"
	"time"
)

var (
	ErrLoanOfferUnavailable     = errors.New("loan offer is not available")
	ErrLoanOfferNotAccepted     = errors.New("loan offer has not been accepted")
	ErrLoanContractNotConfirmed = errors.New("loan contract has not been confirmed")
)

type LoanOfferStatus string

const (
	LoanOfferIssued    LoanOfferStatus = "issued"
	LoanOfferAccepted  LoanOfferStatus = "accepted"
	LoanOfferRejected  LoanOfferStatus = "rejected"
	LoanOfferConfirmed LoanOfferStatus = "confirmed"
)

type LoanOffer struct {
	Amount             int64           `json:"amount"`
	AnnualRateBps      int64           `json:"annualRateBps"`
	TermMonths         int             `json:"termMonths"`
	Status             LoanOfferStatus `json:"status"`
	IssuedAt           time.Time       `json:"issuedAt"`
	AcceptedAt         *time.Time      `json:"acceptedAt,omitempty"`
	RejectedAt         *time.Time      `json:"rejectedAt,omitempty"`
	ConfirmedAt        *time.Time      `json:"confirmedAt,omitempty"`
	ConfirmationMethod string          `json:"confirmationMethod,omitempty"`
}

// Offer terms are frozen once issued and must match the approved request.
func (a *Application) IssueOffer(at time.Time, rate int64) error {
	if a == nil || a.Status != ApplicationStatusApproved || a.ApprovedAt == nil || a.DisbursementPreparedAt != nil || a.Offer != nil {
		return ErrInvalidApplicationStatus
	}
	if at.IsZero() || at.Before(*a.ApprovedAt) || rate <= 0 {
		return ErrLoanOfferUnavailable
	}
	if _, err := CalculateRepaymentSchedule(a.RequestedAmount, rate, a.TermMonths, at); err != nil {
		return err
	}
	a.Offer = &LoanOffer{Amount: a.RequestedAmount, AnnualRateBps: rate, TermMonths: a.TermMonths, Status: LoanOfferIssued, IssuedAt: at}
	a.UpdatedAt = at
	return nil
}

func (a *Application) AcceptOffer(at time.Time) error {
	if a == nil || a.Status != ApplicationStatusApproved || a.Offer == nil || a.Offer.Status != LoanOfferIssued || at.IsZero() || at.Before(a.Offer.IssuedAt) {
		return ErrLoanOfferUnavailable
	}
	a.Offer.Status = LoanOfferAccepted
	a.Offer.AcceptedAt = &at
	a.UpdatedAt = at
	return nil
}

func (a *Application) RejectOffer(at time.Time) error {
	if a == nil || a.Status != ApplicationStatusApproved || a.Offer == nil || a.Offer.Status != LoanOfferIssued || at.IsZero() || at.Before(a.Offer.IssuedAt) {
		return ErrLoanOfferUnavailable
	}
	a.Offer.Status = LoanOfferRejected
	a.Offer.RejectedAt = &at
	a.UpdatedAt = at
	return nil
}

func (a *Application) ConfirmContract(at time.Time, method string) error {
	if a == nil || a.Status != ApplicationStatusApproved || a.Offer == nil || a.Offer.Status != LoanOfferAccepted || a.Offer.AcceptedAt == nil {
		return ErrLoanOfferNotAccepted
	}
	if at.IsZero() || at.Before(*a.Offer.AcceptedAt) || method != "email_otp" {
		return ErrLoanContractNotConfirmed
	}
	a.Offer.Status = LoanOfferConfirmed
	a.Offer.ConfirmedAt = &at
	a.Offer.ConfirmationMethod = method
	a.UpdatedAt = at
	return nil
}
