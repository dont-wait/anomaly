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

// approvedTermRates is the single authoritative rate table in the GĐ1
// simulated savings policy. The package catalog/quote in GĐ2 must use this
// list rather than maintain a second independent rate lookup.
var approvedTermRates = [...]TermRate{
	{Months: 3, AnnualRateBps: 450},
	{Months: 6, AnnualRateBps: 480},
	{Months: 9, AnnualRateBps: 500},
	{Months: 12, AnnualRateBps: 520},
	{Months: 24, AnnualRateBps: 550},
	{Months: 36, AnnualRateBps: 580},
	{Months: 60, AnnualRateBps: 600},
}

// CurrentTermRates returns an independent copy ordered for the API.
func CurrentTermRates() []TermRate {
	return append([]TermRate(nil), approvedTermRates[:]...)
}

func AnnualRateBpsForTerm(months int) (int64, error) {
	for _, term := range approvedTermRates {
		if term.Months == months {
			return term.AnnualRateBps, nil
		}
	}
	return 0, ErrUnsupportedTerm
}

func ValidateDeposit(principal int64, termMonths int) error {
	if principal < MinimumDepositVND {
		return ErrInvalidDepositAmount
	}
	_, err := AnnualRateBpsForTerm(termMonths)
	return err
}
