package loan

import (
	"fmt"
	"math/big"
	"time"
)

type DTIAssessmentStatus string

const (
	DTIStatusPass         DTIAssessmentStatus = "pass"
	DTIStatusManualReview DTIAssessmentStatus = "manual_review"
	DTIStatusReject       DTIAssessmentStatus = "reject"
)

// Kết quả đánh giá sơ bộ khả năng trả nợ.
type LoanAffordability struct {
	IncomeLimit             int64
	WithinIncomeLimit       bool
	EstimatedMonthlyPayment int64
	ExistingMonthlyDebt     int64
	TotalMonthlyDebt        int64
	DTIBps                  int64
	DTIStatus               DTIAssessmentStatus
}

// EvaluateLoanAffordability đánh giá hạn mức và DTI.
//
// monthlyIncome: thu nhập bình quân tháng (VNĐ)
// existingMonthlyDebt: tổng tiền trả nợ hiện tại/tháng (VNĐ)
// requestedAmount: số tiền khách hàng muốn vay (VNĐ)
// annualRateBps: lãi suất năm, ví dụ 1650 = 16.5%
func EvaluateLoanAffordability(
	monthlyIncome int64,
	existingMonthlyDebt int64,
	requestedAmount int64,
	annualRateBps int64,
	termMonths int,
	disbursementDate time.Time,
) (*LoanAffordability, error) {
	// 1. Kiểm tra dữ liệu đầu vào.
	if monthlyIncome <= 0 {
		return nil, fmt.Errorf("monthly income must be greater than zero")
	}

	if existingMonthlyDebt < 0 {
		return nil, fmt.Errorf("existing monthly debt cannot be negative")
	}

	if requestedAmount <= 0 {
		return nil, ErrInvalidAmount
	}

	// 2. Tính hạn mức sơ bộ = 10 lần thu nhập.
	incomeLimitBig := new(big.Int).Mul(
		big.NewInt(monthlyIncome),
		big.NewInt(10),
	)

	if !incomeLimitBig.IsInt64() {
		return nil, fmt.Errorf("income limit exceeds int64")
	}

	incomeLimit := incomeLimitBig.Int64()

	withinIncomeLimit := requestedAmount <= incomeLimit

	// 3. Tính bảng trả nợ dự kiến.
	schedule, err := CalculateRepaymentSchedule(
		requestedAmount,
		annualRateBps,
		termMonths,
		disbursementDate,
	)
	if err != nil {
		return nil, err
	}

	// 4. Lấy khoản thanh toán lớn nhất trong các kỳ.
	// Điều này tránh bỏ sót trường hợp kỳ cuối điều chỉnh.
	var estimatedMonthlyPayment int64

	for _, item := range schedule {
		if item.TotalPayment > estimatedMonthlyPayment {
			estimatedMonthlyPayment = item.TotalPayment
		}
	}

	// 5. Tổng nghĩa vụ trả nợ hằng tháng.
	totalDebtBig := new(big.Int).Add(
		big.NewInt(existingMonthlyDebt),
		big.NewInt(estimatedMonthlyPayment),
	)

	if !totalDebtBig.IsInt64() {
		return nil, fmt.Errorf("total monthly debt exceeds int64")
	}

	totalMonthlyDebt := totalDebtBig.Int64()

	// 6. Tính DTI bằng basis points.
	// 4000 bps = 40%
	// 5000 bps = 50%
	dtiNumerator := new(big.Int).Mul(
		totalDebtBig,
		big.NewInt(10000),
	)

	dtiBps, err := roundPositiveDiv(
		dtiNumerator,
		big.NewInt(monthlyIncome),
	)
	if err != nil {
		return nil, err
	}

	// 7. Phân loại DTI.
	// So sánh tỷ lệ chính xác trước khi làm tròn.
	passThreshold := new(big.Int).Mul(
		big.NewInt(monthlyIncome),
		big.NewInt(4000),
	)

	rejectThreshold := new(big.Int).Mul(
		big.NewInt(monthlyIncome),
		big.NewInt(5000),
	)

	var dtiStatus DTIAssessmentStatus

	switch {
	case dtiNumerator.Cmp(passThreshold) <= 0:
		dtiStatus = DTIStatusPass

	case dtiNumerator.Cmp(rejectThreshold) <= 0:
		dtiStatus = DTIStatusManualReview

	default:
		dtiStatus = DTIStatusReject
	}

	// 8. Trả kết quả đánh giá.
	return &LoanAffordability{
		IncomeLimit:             incomeLimit,
		WithinIncomeLimit:       withinIncomeLimit,
		EstimatedMonthlyPayment: estimatedMonthlyPayment,
		ExistingMonthlyDebt:     existingMonthlyDebt,
		TotalMonthlyDebt:        totalMonthlyDebt,
		DTIBps:                  dtiBps,
		DTIStatus:               dtiStatus,
	}, nil
}
