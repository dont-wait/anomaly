package loan

import (
	"testing"
	"time"
)

// Ngày giải ngân mẫu dùng chung cho các bài test.
func affordabilityTestDate() time.Time {
	return time.Date(
		2026, 10, 15, 0, 0, 0, 0, time.UTC,
	)
}

// Hàm hỗ trợ kiểm thử khả năng trả nợ.
func evaluateTestAffordability(
	t *testing.T,
	income int64,
	existingDebt int64,
	amount int64,
) *LoanAffordability {
	t.Helper()

	result, err := EvaluateLoanAffordability(
		income,
		existingDebt,
		amount,
		1600,
		6,
		affordabilityTestDate(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	return result
}

// Kiểm tra tính hạn mức vay bằng 10 lần thu nhập.
func TestEvaluateLoanAffordability_IncomeLimit(t *testing.T) {
	tests := []struct {
		name          string
		income        int64
		amount        int64
		expectedLimit int64
		withinLimit   bool
	}{
		{
			name:          "below income limit",
			income:        15_000_000,
			amount:        100_000_000,
			expectedLimit: 150_000_000,
			withinLimit:   true,
		},
		{
			name:          "exact income limit",
			income:        15_000_000,
			amount:        150_000_000,
			expectedLimit: 150_000_000,
			withinLimit:   true,
		},
		{
			name:          "above income limit",
			income:        15_000_000,
			amount:        150_000_001,
			expectedLimit: 150_000_000,
			withinLimit:   false,
		},
		{
			name:          "higher income",
			income:        40_000_000,
			amount:        350_000_000,
			expectedLimit: 400_000_000,
			withinLimit:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluateTestAffordability(
				t,
				tt.income,
				0,
				tt.amount,
			)

			if result.IncomeLimit != tt.expectedLimit {
				t.Errorf(
					"expected income limit %d, got %d",
					tt.expectedLimit,
					result.IncomeLimit,
				)
			}

			if result.WithinIncomeLimit != tt.withinLimit {
				t.Errorf(
					"expected within limit %v, got %v",
					tt.withinLimit,
					result.WithinIncomeLimit,
				)
			}
		})
	}
}

// Kiểm tra phân loại DTI tại các ngưỡng chính xác.
// Ví dụ: thu nhập 15 triệu, trả góp mới 1.745.303đ.
func TestEvaluateLoanAffordability_DTIThresholds(t *testing.T) {
	const income int64 = 15_000_000
	const loanAmount int64 = 10_000_000

	// Tính khoản thanh toán lớn nhất từ lịch trả nợ.
	schedule, err := CalculateRepaymentSchedule(
		loanAmount,
		1600,
		6,
		affordabilityTestDate(),
	)
	if err != nil {
		t.Fatalf("failed to calculate schedule: %v", err)
	}

	var monthlyPayment int64

	for _, item := range schedule {
		if item.TotalPayment > monthlyPayment {
			monthlyPayment = item.TotalPayment
		}
	}

	// Với thu nhập 15 triệu:
	// 40% = 6.000.000đ
	// 50% = 7.500.000đ
	tests := []struct {
		name           string
		totalDebt      int64
		expectedStatus DTIAssessmentStatus
		expectedBps    int64
	}{
		{
			name:           "exactly 40 percent",
			totalDebt:      6_000_000,
			expectedStatus: DTIStatusPass,
			expectedBps:    4000,
		},
		{
			name:           "one VND above 40 percent",
			totalDebt:      6_000_001,
			expectedStatus: DTIStatusManualReview,
			expectedBps:    4000,
		},
		{
			name:           "exactly 50 percent",
			totalDebt:      7_500_000,
			expectedStatus: DTIStatusManualReview,
			expectedBps:    5000,
		},
		{
			name:           "one VND above 50 percent",
			totalDebt:      7_500_001,
			expectedStatus: DTIStatusReject,
			expectedBps:    5000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			existingDebt := tt.totalDebt - monthlyPayment

			if existingDebt < 0 {
				t.Fatal("invalid test setup")
			}

			result := evaluateTestAffordability(
				t,
				income,
				existingDebt,
				loanAmount,
			)

			if result.DTIStatus != tt.expectedStatus {
				t.Errorf(
					"expected status %s, got %s",
					tt.expectedStatus,
					result.DTIStatus,
				)
			}

			if result.DTIBps != tt.expectedBps {
				t.Errorf(
					"expected DTI %d bps, got %d",
					tt.expectedBps,
					result.DTIBps,
				)
			}

			if result.TotalMonthlyDebt != tt.totalDebt {
				t.Errorf(
					"expected total debt %d, got %d",
					tt.totalDebt,
					result.TotalMonthlyDebt,
				)
			}

			if result.EstimatedMonthlyPayment != monthlyPayment {
				t.Error("estimated monthly payment mismatch")
			}
		})
	}
}

// Kiểm tra lấy khoản trả góp lớn nhất của lịch dự kiến.
func TestEvaluateLoanAffordability_MonthlyPayment(t *testing.T) {
	result := evaluateTestAffordability(
		t,
		15_000_000,
		2_000_000,
		10_000_000,
	)

	if result.EstimatedMonthlyPayment != 1_745_303 {
		t.Errorf(
			"expected monthly payment 1745303, got %d",
			result.EstimatedMonthlyPayment,
		)
	}

	expectedTotalDebt := int64(3_745_303)

	if result.TotalMonthlyDebt != expectedTotalDebt {
		t.Errorf(
			"expected total debt %d, got %d",
			expectedTotalDebt,
			result.TotalMonthlyDebt,
		)
	}

	if result.DTIStatus != DTIStatusPass {
		t.Errorf(
			"expected DTI pass, got %s",
			result.DTIStatus,
		)
	}

	if !result.WithinIncomeLimit {
		t.Error("loan must be within income limit")
	}
}

// Kiểm tra dữ liệu đầu vào không hợp lệ.
func TestEvaluateLoanAffordability_InvalidInput(t *testing.T) {
	tests := []struct {
		name         string
		income       int64
		existingDebt int64
		amount       int64
		rate         int64
		term         int
		date         time.Time
	}{
		{
			name:   "zero income",
			income: 0,
			amount: 10_000_000,
			rate:   1600,
			term:   6,
			date:   affordabilityTestDate(),
		},
		{
			name:   "negative income",
			income: -1,
			amount: 10_000_000,
			rate:   1600,
			term:   6,
			date:   affordabilityTestDate(),
		},
		{
			name:         "negative existing debt",
			income:       15_000_000,
			existingDebt: -1,
			amount:       10_000_000,
			rate:         1600,
			term:         6,
			date:         affordabilityTestDate(),
		},
		{
			name:   "zero loan amount",
			income: 15_000_000,
			amount: 0,
			rate:   1600,
			term:   6,
			date:   affordabilityTestDate(),
		},
		{
			name:   "negative interest rate",
			income: 15_000_000,
			amount: 10_000_000,
			rate:   -1,
			term:   6,
			date:   affordabilityTestDate(),
		},
		{
			name:   "unsupported term",
			income: 15_000_000,
			amount: 10_000_000,
			rate:   1600,
			term:   7,
			date:   affordabilityTestDate(),
		},
		{
			name:   "missing disbursement date",
			income: 15_000_000,
			amount: 10_000_000,
			rate:   1600,
			term:   6,
		},
		{
			name:   "income limit overflow",
			income: 1 << 62,
			amount: 10_000_000,
			rate:   1600,
			term:   6,
			date:   affordabilityTestDate(),
		},
		{
			name:         "total debt overflow",
			income:       15_000_000,
			existingDebt: 1<<63 - 1,
			amount:       10_000_000,
			rate:         1600,
			term:         6,
			date:         affordabilityTestDate(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := EvaluateLoanAffordability(
				tt.income,
				tt.existingDebt,
				tt.amount,
				tt.rate,
				tt.term,
				tt.date,
			)

			if err == nil {
				t.Fatal("expected validation error")
			}

			if result != nil {
				t.Error("result must be nil on error")
			}
		})
	}
}
