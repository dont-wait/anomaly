package savings

import "time"

const (
	// Product minimum only: no fixed product-level maximum. Int64 and
	// available account balance remain mandatory downstream checks.
	MinimumDepositVND    int64 = 1_000_000
	QuoteTTL                   = 5 * time.Minute
	MaturityPollInterval       = time.Minute
)

// TermRate is the approved AnomalyBank simulated rate for a savings term.
// Bps uses 100 basis points = 1 percent per year.
type TermRate struct {
	Months        int
	AnnualRateBps int64
}

// CurrentTermRates returns a copy in the order displayed by the API.
// Package/catalog infrastructure will persist and serve these terms in GĐ2.
func CurrentTermRates() []TermRate {
	return []TermRate{
		{Months: 3, AnnualRateBps: 450},
		{Months: 6, AnnualRateBps: 480},
		{Months: 9, AnnualRateBps: 500},
		{Months: 12, AnnualRateBps: 520},
		{Months: 24, AnnualRateBps: 550},
		{Months: 36, AnnualRateBps: 580},
		{Months: 60, AnnualRateBps: 600},
	}
}

func AnnualRateBpsForTerm(months int) (int64, error) {
	switch months {
	case 3:
		return 450, nil
	case 6:
		return 480, nil
	case 9:
		return 500, nil
	case 12:
		return 520, nil
	case 24:
		return 550, nil
	case 36:
		return 580, nil
	case 60:
		return 600, nil
	default:
		return 0, ErrUnsupportedTerm
	}
}

func ValidateDeposit(principal int64, termMonths int) error {
	if principal < MinimumDepositVND {
		return ErrInvalidDepositAmount
	}
	_, err := AnnualRateBpsForTerm(termMonths)
	return err
}
