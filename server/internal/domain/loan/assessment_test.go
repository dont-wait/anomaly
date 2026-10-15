package loan

import "testing"

// Tạo dữ liệu thẩm định hợp lệ cho các Unit Test.
// DTI = 35%, BHXH = 24 tháng, lịch sử trả nợ đúng hạn.
// Tổng điểm mong đợi = 30 + 25 + 35 = 90.
func validAssessmentTestInput() LoanAssessmentInput {
	return LoanAssessmentInput{
		MonthlyIncome: 10_000_000,
		Affordability: &LoanAffordability{
			TotalMonthlyDebt:  3_500_000,
			WithinIncomeLimit: true,
			DTIBps:            3500,
			DTIStatus:         DTIStatusPass,
		},
		InsuranceMonths:       24,
		InsuranceVerified:     true,
		RepaymentHistory:      HistoryOnTime,
		HistoryVerified:       true,
		FinancialDataVerified: true,
		DocumentsVerified:     true,
	}
}

// Kiểm tra hồ sơ đủ điều kiện xét duyệt tự động.
func TestAssessLoanApplication_AutoEligible(t *testing.T) {
	input := validAssessmentTestInput()

	result, err := AssessLoanApplication(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.DTIScore != 30 {
		t.Errorf("expected DTI score 30, got %d", result.DTIScore)
	}

	if result.InsuranceScore != 25 {
		t.Errorf("expected insurance score 25, got %d", result.InsuranceScore)
	}

	if result.HistoryScore != 35 {
		t.Errorf("expected history score 35, got %d", result.HistoryScore)
	}

	if result.TotalScore != 90 {
		t.Errorf("expected total score 90, got %d", result.TotalScore)
	}

	if result.Decision != AssessmentAutoEligible {
		t.Errorf("unexpected decision: %s", result.Decision)
	}

	if len(result.Reasons) != 0 {
		t.Errorf("expected no reasons, got %v", result.Reasons)
	}
}

// Kiểm tra các ngưỡng DTI chính xác.
func TestAssessLoanApplication_DTIThresholds(t *testing.T) {
	tests := []struct {
		name          string
		monthlyDebt   int64
		expectedScore int
		expected      LoanAssessmentDecision
	}{
		{
			name:          "exactly 30 percent",
			monthlyDebt:   3_000_000,
			expectedScore: 40,
			expected:      AssessmentAutoEligible,
		},
		{
			name:          "above 30 percent",
			monthlyDebt:   3_000_001,
			expectedScore: 30,
			expected:      AssessmentAutoEligible,
		},
		{
			name:          "exactly 40 percent",
			monthlyDebt:   4_000_000,
			expectedScore: 30,
			expected:      AssessmentAutoEligible,
		},
		{
			name:          "above 40 percent",
			monthlyDebt:   4_000_001,
			expectedScore: 15,
			expected:      AssessmentManualReview,
		},
		{
			name:          "exactly 50 percent",
			monthlyDebt:   5_000_000,
			expectedScore: 15,
			expected:      AssessmentManualReview,
		},
		{
			name:          "above 50 percent",
			monthlyDebt:   5_000_001,
			expectedScore: 0,
			expected:      AssessmentRejected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validAssessmentTestInput()
			input.Affordability.TotalMonthlyDebt = tt.monthlyDebt

			result, err := AssessLoanApplication(input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.DTIScore != tt.expectedScore {
				t.Errorf(
					"expected DTI score %d, got %d",
					tt.expectedScore, result.DTIScore,
				)
			}

			if result.Decision != tt.expected {
				t.Errorf(
					"expected %s, got %s",
					tt.expected, result.Decision,
				)
			}
		})
	}
}

// Kiểm tra thời gian tham gia BHXH.
func TestAssessLoanApplication_InsuranceMonths(t *testing.T) {
	tests := []struct {
		months        int
		expectedScore int
		expected      LoanAssessmentDecision
	}{
		{5, 0, AssessmentRejected},
		{6, 10, AssessmentManualReview},
		{11, 10, AssessmentManualReview},
		{12, 20, AssessmentAutoEligible},
		{23, 20, AssessmentAutoEligible},
		{24, 25, AssessmentAutoEligible},
	}

	for _, tt := range tests {
		input := validAssessmentTestInput()
		input.InsuranceMonths = tt.months

		result, err := AssessLoanApplication(input)
		if err != nil {
			t.Fatalf("months %d: %v", tt.months, err)
		}

		if result.InsuranceScore != tt.expectedScore {
			t.Errorf(
				"months %d: expected score %d, got %d",
				tt.months, tt.expectedScore, result.InsuranceScore,
			)
		}

		if result.Decision != tt.expected {
			t.Errorf(
				"months %d: expected %s, got %s",
				tt.months, tt.expected, result.Decision,
			)
		}
	}
}

// Kiểm tra lịch sử trả nợ.
func TestAssessLoanApplication_RepaymentHistory(t *testing.T) {
	tests := []struct {
		name          string
		history       RepaymentHistoryStatus
		expectedScore int
		expected      LoanAssessmentDecision
	}{
		{"on time", HistoryOnTime, 35, AssessmentAutoEligible},
		{"never borrowed", HistoryNever, 30, AssessmentAutoEligible},
		{"previously late", HistoryLate, 15, AssessmentManualReview},
		{"overdue", HistoryOverdue, 0, AssessmentRejected},
		{"unknown", HistoryUnknown, 0, AssessmentManualReview},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validAssessmentTestInput()
			input.RepaymentHistory = tt.history

			result, err := AssessLoanApplication(input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.HistoryScore != tt.expectedScore {
				t.Errorf(
					"expected history score %d, got %d",
					tt.expectedScore, result.HistoryScore,
				)
			}

			if result.Decision != tt.expected {
				t.Errorf(
					"expected %s, got %s",
					tt.expected, result.Decision,
				)
			}
		})
	}
}

// Kiểm tra thông tin chưa xác minh và hạn mức vay.
func TestAssessLoanApplication_VerificationAndLimit(t *testing.T) {
	tests := []struct {
		name     string
		modify   func(*LoanAssessmentInput)
		expected LoanAssessmentDecision
	}{
		{
			name: "financial data not verified",
			modify: func(i *LoanAssessmentInput) {
				i.FinancialDataVerified = false
			},
			expected: AssessmentManualReview,
		},
		{
			name: "insurance not verified",
			modify: func(i *LoanAssessmentInput) {
				i.InsuranceVerified = false
			},
			expected: AssessmentManualReview,
		},
		{
			name: "history not verified",
			modify: func(i *LoanAssessmentInput) {
				i.HistoryVerified = false
			},
			expected: AssessmentManualReview,
		},
		{
			name: "documents not verified",
			modify: func(i *LoanAssessmentInput) {
				i.DocumentsVerified = false
			},
			expected: AssessmentManualReview,
		},
		{
			name: "exceeds income limit",
			modify: func(i *LoanAssessmentInput) {
				i.Affordability.WithinIncomeLimit = false
			},
			expected: AssessmentRejected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validAssessmentTestInput()
			tt.modify(&input)

			result, err := AssessLoanApplication(input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Decision != tt.expected {
				t.Errorf(
					"expected %s, got %s",
					tt.expected, result.Decision,
				)
			}
		})
	}
}

// Kiểm tra dữ liệu không hợp lệ.
func TestAssessLoanApplication_InvalidInput(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*LoanAssessmentInput)
	}{
		{
			name: "zero income",
			modify: func(i *LoanAssessmentInput) {
				i.MonthlyIncome = 0
			},
		},
		{
			name: "missing affordability",
			modify: func(i *LoanAssessmentInput) {
				i.Affordability = nil
			},
		},
		{
			name: "negative debt",
			modify: func(i *LoanAssessmentInput) {
				i.Affordability.TotalMonthlyDebt = -1
			},
		},
		{
			name: "negative insurance months",
			modify: func(i *LoanAssessmentInput) {
				i.InsuranceMonths = -1
			},
		},
		{
			name: "invalid repayment history",
			modify: func(i *LoanAssessmentInput) {
				i.RepaymentHistory = "invalid"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validAssessmentTestInput()
			tt.modify(&input)

			result, err := AssessLoanApplication(input)

			if err == nil {
				t.Fatal("expected validation error")
			}

			if result != nil {
				t.Error("result must be nil on error")
			}
		})
	}
}
