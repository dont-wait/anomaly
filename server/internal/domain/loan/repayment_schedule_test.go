package loan

import (
	"testing"
	"time"
)

// Kiểm tra lịch trả nợ 10 triệu trong 6 tháng,
// lãi suất cố định 16%/năm.
func TestCalculateRepaymentSchedule_Success(t *testing.T) {
	disbursementDate := time.Date(
		2026, 10, 15, 0, 0, 0, 0, time.UTC,
	)

	schedule, err := CalculateRepaymentSchedule(
		10_000_000,
		1600,
		6,
		disbursementDate,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(schedule) != 6 {
		t.Fatalf("expected 6 installments, got %d", len(schedule))
	}

	first := schedule[0]

	if first.DueDate.Format("2006-01-02") != "2026-11-15" {
		t.Errorf("unexpected first due date: %v", first.DueDate)
	}

	if first.TotalPayment != 1_745_303 {
		t.Errorf(
			"expected first payment 1745303, got %d",
			first.TotalPayment,
		)
	}

	if first.PrincipalAmount != 1_611_970 {
		t.Errorf(
			"unexpected first principal: %d",
			first.PrincipalAmount,
		)
	}

	if first.InterestAmount != 133_333 {
		t.Errorf(
			"unexpected first interest: %d",
			first.InterestAmount,
		)
	}

	var totalPrincipal int64
	var totalInterest int64

	previousBalance := int64(10_000_000)

	for i, item := range schedule {
		if item.InstallmentNo != i+1 {
			t.Errorf("invalid installment number at %d", i)
		}

		if item.OpeningBalance != previousBalance {
			t.Errorf("opening balance mismatch at installment %d", i+1)
		}

		if item.TotalPayment !=
			item.PrincipalAmount+item.InterestAmount {
			t.Errorf("payment mismatch at installment %d", i+1)
		}

		if item.RemainingBalance !=
			item.OpeningBalance-item.PrincipalAmount {
			t.Errorf("remaining balance mismatch at installment %d", i+1)
		}

		if item.PrincipalAmount <= 0 || item.InterestAmount < 0 {
			t.Errorf("invalid amounts at installment %d", i+1)
		}

		totalPrincipal += item.PrincipalAmount
		totalInterest += item.InterestAmount
		previousBalance = item.RemainingBalance
	}

	if totalPrincipal != 10_000_000 {
		t.Errorf(
			"expected total principal 10000000, got %d",
			totalPrincipal,
		)
	}

	if totalInterest != 471_816 {
		t.Errorf(
			"expected total interest 471816, got %d",
			totalInterest,
		)
	}

	last := schedule[len(schedule)-1]

	if last.RemainingBalance != 0 {
		t.Errorf(
			"expected remaining balance 0, got %d",
			last.RemainingBalance,
		)
	}

	if last.DueDate.Format("2006-01-02") != "2027-04-15" {
		t.Errorf("unexpected last due date: %v", last.DueDate)
	}
}

// Kiểm tra tất cả kỳ hạn được hỗ trợ.
func TestCalculateRepaymentSchedule_SupportedTerms(t *testing.T) {
	terms := []int{6, 12, 24, 36, 48, 60}

	for _, term := range terms {
		t.Run(
			time.Duration(term).String(),
			func(t *testing.T) {
				schedule, err := CalculateRepaymentSchedule(
					100_000_000,
					1600,
					term,
					time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC),
				)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				if len(schedule) != term {
					t.Errorf(
						"expected %d installments, got %d",
						term, len(schedule),
					)
				}

				if schedule[len(schedule)-1].RemainingBalance != 0 {
					t.Error("loan must be fully repaid")
				}
			},
		)
	}
}

// Kiểm tra trường hợp lãi suất 0%.
func TestCalculateRepaymentSchedule_ZeroInterest(t *testing.T) {
	schedule, err := CalculateRepaymentSchedule(
		12_000_000,
		0,
		12,
		time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, item := range schedule {
		if item.InterestAmount != 0 {
			t.Error("interest must be zero")
		}

		if item.TotalPayment != 1_000_000 {
			t.Errorf("expected 1000000, got %d", item.TotalPayment)
		}
	}

	if schedule[11].RemainingBalance != 0 {
		t.Error("remaining balance must be zero")
	}
}

// Kiểm tra ngày trả nợ của các tháng có 28, 29, 30, 31 ngày.
func TestCalculateDueDate_MonthEnd(t *testing.T) {
	tests := []struct {
		name     string
		start    time.Time
		month    int
		expected string
	}{
		{
			name: "January 28",
			start: time.Date(
				2027, 1, 28, 0, 0, 0, 0, time.UTC,
			),
			month:    1,
			expected: "2027-02-28",
		},
		{
			name: "January 29 non-leap year",
			start: time.Date(
				2027, 1, 29, 0, 0, 0, 0, time.UTC,
			),
			month:    1,
			expected: "2027-02-28",
		},
		{
			name: "January 30",
			start: time.Date(
				2027, 1, 30, 0, 0, 0, 0, time.UTC,
			),
			month:    1,
			expected: "2027-02-28",
		},
		{
			name: "January 31",
			start: time.Date(
				2027, 1, 31, 0, 0, 0, 0, time.UTC,
			),
			month:    1,
			expected: "2027-02-28",
		},
		{
			name: "Leap year February 29",
			start: time.Date(
				2028, 1, 31, 0, 0, 0, 0, time.UTC,
			),
			month:    1,
			expected: "2028-02-29",
		},
		{
			name: "March returns to day 31",
			start: time.Date(
				2027, 1, 31, 0, 0, 0, 0, time.UTC,
			),
			month:    2,
			expected: "2027-03-31",
		},
		{
			name: "April has 30 days",
			start: time.Date(
				2027, 1, 31, 0, 0, 0, 0, time.UTC,
			),
			month:    3,
			expected: "2027-04-30",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateDueDate(tt.start, tt.month)

			if result.Format("2006-01-02") != tt.expected {
				t.Errorf(
					"expected %s, got %s",
					tt.expected,
					result.Format("2006-01-02"),
				)
			}
		})
	}
}

// Kiểm tra dữ liệu đầu vào không hợp lệ.
func TestCalculateRepaymentSchedule_InvalidInput(t *testing.T) {
	validDate := time.Date(
		2026, 10, 15, 0, 0, 0, 0, time.UTC,
	)

	tests := []struct {
		name      string
		principal int64
		rate      int64
		term      int
		date      time.Time
	}{
		{"zero principal", 0, 1600, 12, validDate},
		{"negative principal", -100, 1600, 12, validDate},
		{"negative rate", 10_000_000, -100, 12, validDate},
		{"unsupported term", 10_000_000, 1600, 7, validDate},
		{"zero term", 10_000_000, 1600, 0, validDate},
		{"missing date", 10_000_000, 1600, 12, time.Time{}},
		{"amount too small", 1, 1600, 60, validDate},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schedule, err := CalculateRepaymentSchedule(
				tt.principal,
				tt.rate,
				tt.term,
				tt.date,
			)

			if err == nil {
				t.Error("expected validation error")
			}

			if schedule != nil {
				t.Error("schedule must be nil on error")
			}
		})
	}
}
