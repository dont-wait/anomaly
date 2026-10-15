package loan

import (
	"fmt"
	"math/big"
)

// Phân loại lịch sử trả nợ.
type RepaymentHistoryStatus string

const (
	HistoryOnTime  RepaymentHistoryStatus = "on_time"
	HistoryNever   RepaymentHistoryStatus = "never_borrowed"
	HistoryLate    RepaymentHistoryStatus = "previously_late"
	HistoryOverdue RepaymentHistoryStatus = "overdue"
	HistoryUnknown RepaymentHistoryStatus = "unknown"
)

// Kết quả phân luồng thẩm định.
type LoanAssessmentDecision string

const (
	AssessmentAutoEligible LoanAssessmentDecision = "auto_approve_eligible"
	AssessmentManualReview LoanAssessmentDecision = "manual_review"
	AssessmentRejected     LoanAssessmentDecision = "rejected"
)

// Dữ liệu đầu vào phục vụ chấm điểm.
type LoanAssessmentInput struct {
	MonthlyIncome         int64
	Affordability         *LoanAffordability
	InsuranceMonths       int
	InsuranceVerified     bool
	RepaymentHistory      RepaymentHistoryStatus
	HistoryVerified       bool
	FinancialDataVerified bool
	DocumentsVerified     bool
}

// Kết quả chấm điểm hồ sơ.
type LoanAssessmentResult struct {
	DTIScore       int
	InsuranceScore int
	HistoryScore   int
	TotalScore     int
	Decision       LoanAssessmentDecision
	Reasons        []string
}

// AssessLoanApplication chấm điểm hồ sơ theo chính sách nội bộ.
// Không sử dụng mô hình Machine Learning.
func AssessLoanApplication(
	input LoanAssessmentInput,
) (*LoanAssessmentResult, error) {
	// 1. Kiểm tra dữ liệu đầu vào.
	if input.MonthlyIncome <= 0 {
		return nil, fmt.Errorf("monthly income must be positive")
	}

	if input.Affordability == nil {
		return nil, fmt.Errorf("loan affordability is required")
	}

	if input.Affordability.TotalMonthlyDebt < 0 {
		return nil, fmt.Errorf("total monthly debt cannot be negative")
	}

	if input.InsuranceMonths < 0 {
		return nil, fmt.Errorf("insurance months cannot be negative")
	}

	switch input.RepaymentHistory {
	case HistoryOnTime, HistoryNever, HistoryLate,
		HistoryOverdue, HistoryUnknown:
	default:
		return nil, fmt.Errorf("invalid repayment history status")
	}

	result := &LoanAssessmentResult{
		Reasons: []string{},
	}

	rejected := false
	manualReview := false

	// 2. Tính điểm DTI bằng so sánh số nguyên chính xác.
	// DTI <= 30%: 40 điểm
	// 30% < DTI <= 40%: 30 điểm
	// 40% < DTI <= 50%: 15 điểm
	// DTI > 50%: từ chối nếu dữ liệu đã xác minh.

	debt100 := new(big.Int).Mul(
		big.NewInt(input.Affordability.TotalMonthlyDebt),
		big.NewInt(100),
	)

	income30 := new(big.Int).Mul(
		big.NewInt(input.MonthlyIncome),
		big.NewInt(30),
	)

	income40 := new(big.Int).Mul(
		big.NewInt(input.MonthlyIncome),
		big.NewInt(40),
	)

	income50 := new(big.Int).Mul(
		big.NewInt(input.MonthlyIncome),
		big.NewInt(50),
	)

	switch {
	case debt100.Cmp(income30) <= 0:
		result.DTIScore = 40

	case debt100.Cmp(income40) <= 0:
		result.DTIScore = 30

	case debt100.Cmp(income50) <= 0:
		result.DTIScore = 15
		manualReview = true
		result.Reasons = append(
			result.Reasons,
			"DTI requires manual review",
		)

	default:
		if input.FinancialDataVerified {
			rejected = true
			result.Reasons = append(
				result.Reasons,
				"DTI exceeds 50 percent",
			)
		} else {
			manualReview = true
		}
	}

	// 3. Kiểm tra hạn mức 10 lần thu nhập.
	if !input.Affordability.WithinIncomeLimit {
		if input.FinancialDataVerified {
			rejected = true
			result.Reasons = append(
				result.Reasons,
				"requested amount exceeds income limit",
			)
		} else {
			manualReview = true
		}
	}

	if !input.FinancialDataVerified {
		manualReview = true
		result.Reasons = append(
			result.Reasons,
			"financial data is not verified",
		)
	}

	// 4. Chấm điểm thời gian tham gia BHXH.
	if !input.InsuranceVerified {
		manualReview = true
		result.Reasons = append(
			result.Reasons,
			"insurance information is not verified",
		)
	} else {
		switch {
		case input.InsuranceMonths >= 24:
			result.InsuranceScore = 25

		case input.InsuranceMonths >= 12:
			result.InsuranceScore = 20

		case input.InsuranceMonths >= 6:
			result.InsuranceScore = 10
			manualReview = true
			result.Reasons = append(
				result.Reasons,
				"insurance duration requires manual review",
			)

		default:
			rejected = true
			result.Reasons = append(
				result.Reasons,
				"insurance duration below 6 months",
			)
		}
	}

	// 5. Chấm điểm lịch sử trả nợ.
	if !input.HistoryVerified ||
		input.RepaymentHistory == HistoryUnknown {
		manualReview = true
		result.Reasons = append(
			result.Reasons,
			"repayment history is not verified",
		)
	} else {
		switch input.RepaymentHistory {
		case HistoryOnTime:
			result.HistoryScore = 35

		case HistoryNever:
			result.HistoryScore = 30

		case HistoryLate:
			result.HistoryScore = 15
			manualReview = true
			result.Reasons = append(
				result.Reasons,
				"previous late repayment requires manual review",
			)

		case HistoryOverdue:
			rejected = true
			result.Reasons = append(
				result.Reasons,
				"outstanding overdue debt",
			)
		}
	}

	// 6. Kiểm tra trạng thái xác minh chứng từ.
	if !input.DocumentsVerified {
		manualReview = true
		result.Reasons = append(
			result.Reasons,
			"documents are not verified",
		)
	}

	// 7. Tổng hợp điểm.
	result.TotalScore = result.DTIScore +
		result.InsuranceScore +
		result.HistoryScore

	// 8. Quyết định phân luồng.
	switch {
	case rejected:
		result.Decision = AssessmentRejected

	case manualReview || result.TotalScore < 80:
		result.Decision = AssessmentManualReview

	default:
		result.Decision = AssessmentAutoEligible
	}

	return result, nil
}
