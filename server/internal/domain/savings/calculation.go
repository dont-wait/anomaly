package savings

import (
	"math/big"
	"time"

	"github.com/dont-wait/anomaly/internal/timeutil"
)

const annualInterestDenominator int64 = 10_000 * 365 // bps * actual days / 365

// CalculateMaturityAt adds the product term in calendar months in Vietnam's
// business timezone, preserving the opening wall-clock time. If the target
// month lacks the opening day, its last day is used instead. time.AddDate alone
// does not implement this business rule (e.g. Jan 31 + 3 months).
func CalculateMaturityAt(openedAt time.Time, termMonths int) (time.Time, error) {
	if openedAt.IsZero() {
		return time.Time{}, ErrInvalidOpenedAt
	}
	if _, err := AnnualRateBpsForTerm(termMonths); err != nil {
		return time.Time{}, err
	}

	start := openedAt.In(timeutil.VietnamLocation)
	first := time.Date(start.Year(), start.Month()+time.Month(termMonths), 1,
		0, 0, 0, 0, timeutil.VietnamLocation)
	lastDay := first.AddDate(0, 1, -1).Day()
	day := start.Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(first.Year(), first.Month(), day, start.Hour(),
		start.Minute(), start.Second(), start.Nanosecond(), timeutil.VietnamLocation), nil
}

// ActualCalendarDays counts Vietnam-local calendar dates, not elapsed
// 24-hour windows, so partial clock offsets never change the day count.
// The caller's maturity must already be at or after the opening date.
func ActualCalendarDays(openedAt, maturityAt time.Time) (int64, error) {
	if openedAt.IsZero() {
		return 0, ErrInvalidOpenedAt
	}
	if maturityAt.IsZero() || !maturityAt.After(openedAt) {
		return 0, ErrInvalidMaturity
	}
	start := openedAt.In(timeutil.VietnamLocation)
	end := maturityAt.In(timeutil.VietnamLocation)
	startDate := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	endDate := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	days := int64(endDate.Sub(startDate) / (24 * time.Hour))
	if days <= 0 {
		return 0, ErrInvalidMaturity
	}
	return days, nil
}

// CalculateMaturityInterest uses simple interest:
//
//	principal (VND) * annualRateBps * calendarDays / (10000 * 365).
//
// An integer big-number intermediate avoids overflow and float error.
// ROUND_HALF_UP is applied ONCE at the final VND result.
func CalculateMaturityInterest(principal, annualRateBps int64, openedAt, maturityAt time.Time) (int64, error) {
	if principal < MinimumDepositVND {
		return 0, ErrInvalidDepositAmount
	}
	if annualRateBps <= 0 || annualRateBps > 10_000 {
		return 0, ErrInvalidInterestRate
	}
	days, err := ActualCalendarDays(openedAt, maturityAt)
	if err != nil {
		return 0, err
	}

	numerator := new(big.Int).Mul(big.NewInt(principal), big.NewInt(annualRateBps))
	numerator.Mul(numerator, big.NewInt(days))
	denominator := big.NewInt(annualInterestDenominator)
	// All values are nonnegative: adding half of the denominator is
	// equivalent to HALF_UP (at exactly 0.5 VND, round upward).
	result := new(big.Int).Quo(new(big.Int).Add(numerator,
		new(big.Int).Quo(denominator, big.NewInt(2))), denominator)
	if !result.IsInt64() {
		return 0, ErrAmountOverflow
	}
	return result.Int64(), nil
}

func CalculateMaturityPayout(principal, annualRateBps int64, openedAt, maturityAt time.Time) (interest, total int64, err error) {
	interest, err = CalculateMaturityInterest(principal, annualRateBps, openedAt, maturityAt)
	if err != nil {
		return 0, 0, err
	}
	if principal > int64(^uint64(0)>>1)-interest {
		return 0, 0, ErrAmountOverflow
	}
	return interest, principal + interest, nil
}
