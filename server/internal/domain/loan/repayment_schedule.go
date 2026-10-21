package loan

import (
	"fmt"
	"math/big"
	"time"

	"github.com/dont-wait/anomaly/internal/timeutil"
)

const monthlyRateDenominator int64 = 120000

// Một kỳ thanh toán trong lịch trả nợ.
type RepaymentScheduleItem struct {
	InstallmentNo    int
	DueDate          time.Time
	OpeningBalance   int64
	PrincipalAmount  int64
	InterestAmount   int64
	TotalPayment     int64
	RemainingBalance int64
}

// CalculateRepaymentSchedule tính lịch trả nợ dự kiến.
// annualRateBps: lãi suất năm theo basis points.
// Ví dụ: 16%/năm = 1600 basis points.
func CalculateRepaymentSchedule(
	principal int64,
	annualRateBps int64,
	termMonths int,
	disbursementDate time.Time,
) ([]RepaymentScheduleItem, error) {
	if principal <= 0 {
		return nil, ErrInvalidAmount
	}

	switch termMonths {
	case 6, 12, 24, 36, 48, 60:
	default:
		return nil, ErrUnsupportedTerm
	}

	if annualRateBps < 0 {
		return nil, fmt.Errorf("annual interest rate cannot be negative")
	}

	if disbursementDate.IsZero() {
		return nil, fmt.Errorf("disbursement date is required")
	}

	monthlyPayment, err := calculateMonthlyPayment(
		principal, annualRateBps, termMonths,
	)
	if err != nil {
		return nil, err
	}

	remaining := principal
	schedule := make([]RepaymentScheduleItem, 0, termMonths)

	for i := 1; i <= termMonths; i++ {
		openingBalance := remaining

		interest, err := roundPositiveDiv(
			new(big.Int).Mul(
				big.NewInt(remaining),
				big.NewInt(annualRateBps),
			),
			big.NewInt(monthlyRateDenominator),
		)
		if err != nil {
			return nil, err
		}

		principalPayment := monthlyPayment - interest

		// Kỳ cuối thanh toán toàn bộ dư nợ còn lại.
		if i == termMonths {
			principalPayment = remaining
		} else if principalPayment <= 0 ||
			principalPayment >= remaining {
			return nil, fmt.Errorf(
				"loan amount too small for selected term",
			)
		}

		totalPayment := principalPayment + interest
		remaining -= principalPayment

		schedule = append(schedule, RepaymentScheduleItem{
			InstallmentNo:    i,
			DueDate:          calculateDueDate(disbursementDate, i),
			OpeningBalance:   openingBalance,
			PrincipalAmount:  principalPayment,
			InterestAmount:   interest,
			TotalPayment:     totalPayment,
			RemainingBalance: remaining,
		})
	}

	return schedule, nil
}

// Tính tiền trả góp đều hằng tháng.
// Sử dụng số nguyên lớn để hạn chế sai số tiền tệ.
func calculateMonthlyPayment(
	principal, annualRateBps int64,
	termMonths int,
) (int64, error) {
	if annualRateBps == 0 {
		return roundPositiveDiv(
			big.NewInt(principal),
			big.NewInt(int64(termMonths)),
		)
	}

	denominator := big.NewInt(monthlyRateDenominator)
	exponent := big.NewInt(int64(termMonths))

	basePower := new(big.Int).Exp(
		denominator, exponent, nil,
	)

	rateBase := new(big.Int).Add(
		denominator, big.NewInt(annualRateBps),
	)

	ratePower := new(big.Int).Exp(
		rateBase, exponent, nil,
	)

	numerator := new(big.Int).Mul(
		big.NewInt(principal),
		big.NewInt(annualRateBps),
	)
	numerator.Mul(numerator, ratePower)

	divisor := new(big.Int).Sub(ratePower, basePower)
	divisor.Mul(divisor, denominator)

	return roundPositiveDiv(numerator, divisor)
}

// Chia và làm tròn số nguyên dương đến VNĐ gần nhất.
func roundPositiveDiv(
	numerator, denominator *big.Int,
) (int64, error) {
	if numerator.Sign() < 0 || denominator.Sign() <= 0 {
		return 0, fmt.Errorf("invalid division operands")
	}

	twiceNumerator := new(big.Int).Mul(
		numerator, big.NewInt(2),
	)

	roundedNumerator := new(big.Int).Add(
		twiceNumerator, denominator,
	)

	twiceDenominator := new(big.Int).Mul(
		denominator, big.NewInt(2),
	)

	result := new(big.Int).Quo(
		roundedNumerator, twiceDenominator,
	)

	if !result.IsInt64() {
		return 0, fmt.Errorf("calculated amount exceeds int64")
	}

	return result.Int64(), nil
}

// Xác định ngày đến hạn dựa trên ngày giải ngân gốc.
func calculateDueDate(
	disbursementDate time.Time,
	installmentNo int,
) time.Time {
	vn := timeutil.VietnamLocation

	year, month, originalDay := disbursementDate.In(vn).Date()

	firstDay := time.Date(
		year,
		month+time.Month(installmentNo),
		1, 0, 0, 0, 0,
		time.UTC,
	)

	lastDay := firstDay.AddDate(0, 1, -1).Day()

	dueDay := originalDay
	if dueDay > lastDay {
		dueDay = lastDay
	}

	return time.Date(
		firstDay.Year(),
		firstDay.Month(),
		dueDay,
		0, 0, 0, 0,
		time.UTC,
	)
}
