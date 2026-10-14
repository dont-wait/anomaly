package loan

import "testing"

// Kiểm tra lãi suất tại các ngưỡng thu nhập.
func TestCalculateAnnualLoanRateBps_IncomeBoundaries(t *testing.T) {
	tests := []struct {
		name        string
		income      int64
		expectedBps int64
	}{
		{"below 7 million", 6_999_999, 1790},
		{"exactly 7 million", 7_000_000, 1730},
		{"below 10 million", 9_999_999, 1730},
		{"exactly 10 million", 10_000_000, 1670},
		{"below 15 million", 14_999_999, 1670},
		{"exactly 15 million", 15_000_000, 1610},
		{"below 20 million", 19_999_999, 1610},
		{"exactly 20 million", 20_000_000, 1540},
		{"below 30 million", 29_999_999, 1540},
		{"exactly 30 million", 30_000_000, 1480},
		{"above 30 million", 50_000_000, 1480},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rate, err := CalculateAnnualLoanRateBps(
				tt.income,
				"KV01",
				12,
			)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if rate != tt.expectedBps {
				t.Errorf(
					"expected %d bps, got %d bps",
					tt.expectedBps,
					rate,
				)
			}
		})
	}
}

// Kiểm tra điều chỉnh lãi suất theo khu vực.
func TestCalculateAnnualLoanRateBps_Regions(t *testing.T) {
	tests := []struct {
		region      string
		expectedBps int64
	}{
		{"KV01", 1670},
		{"KV02", 1650},
		{"KV03", 1630},
	}

	for _, tt := range tests {
		t.Run(tt.region, func(t *testing.T) {
			rate, err := CalculateAnnualLoanRateBps(
				12_000_000,
				tt.region,
				12,
			)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if rate != tt.expectedBps {
				t.Errorf(
					"expected %d bps, got %d bps",
					tt.expectedBps,
					rate,
				)
			}
		})
	}
}

// Kiểm tra điều chỉnh lãi suất theo kỳ hạn.
func TestCalculateAnnualLoanRateBps_Terms(t *testing.T) {
	tests := []struct {
		name        string
		term        int
		expectedBps int64
	}{
		{"6 months", 6, 1640},
		{"12 months", 12, 1670},
		{"24 months", 24, 1690},
		{"36 months", 36, 1710},
		{"48 months", 48, 1730},
		{"60 months", 60, 1750},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rate, err := CalculateAnnualLoanRateBps(
				12_000_000,
				"KV01",
				tt.term,
			)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if rate != tt.expectedBps {
				t.Errorf(
					"expected %d bps, got %d bps",
					tt.expectedBps,
					rate,
				)
			}
		})
	}
}

// Kiểm tra kết hợp thu nhập, khu vực và kỳ hạn.
// Kiểm tra kết hợp thu nhập, khu vực và kỳ hạn.
func TestCalculateAnnualLoanRateBps_Combined(t *testing.T) {
	// Lương 30 triệu, KV03, kỳ hạn 60 tháng.
	// 14.4% + 0.0% + 0.8% = 15.2%/năm.
	rate, err := CalculateAnnualLoanRateBps(
		30_000_000,
		"KV03",
		60,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rate != 1520 {
		t.Errorf("expected 1520 bps, got %d bps", rate)
	}
}

// Kiểm tra dữ liệu đầu vào không hợp lệ.
func TestCalculateAnnualLoanRateBps_InvalidInput(t *testing.T) {
	tests := []struct {
		name   string
		income int64
		region string
		term   int
	}{
		{"zero income", 0, "KV01", 12},
		{"negative income", -1, "KV01", 12},
		{"empty region", 12_000_000, "", 12},
		{"invalid region", 12_000_000, "KV04", 12},
		{"zero term", 12_000_000, "KV01", 0},
		{"unsupported term", 12_000_000, "KV01", 7},
		{"term over 60 months", 12_000_000, "KV01", 72},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rate, err := CalculateAnnualLoanRateBps(
				tt.income,
				tt.region,
				tt.term,
			)

			if err == nil {
				t.Fatal("expected validation error")
			}

			if rate != 0 {
				t.Errorf("expected rate 0, got %d", rate)
			}
		})
	}
}
