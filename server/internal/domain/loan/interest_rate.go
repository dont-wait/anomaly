package loan

import "fmt"

// CalculateAnnualLoanRateBps xác định lãi suất vay theo
// thu nhập bình quân tháng, khu vực và kỳ hạn.
//
// Giá trị trả về sử dụng basis points:
// 1600 = 16%/năm
// 1630 = 16.3%/năm
func CalculateAnnualLoanRateBps(
	monthlyIncome int64,
	regionCode string,
	termMonths int,
) (int64, error) {

	// 1. Kiểm tra thu nhập.
	if monthlyIncome <= 0 {
		return 0, fmt.Errorf(
			"average monthly income must be greater than zero",
		)
	}

	// 2. Xác định mức điều chỉnh theo khu vực.
	var regionAdjustmentBps int64

	switch regionCode {
	case "KV01":
		regionAdjustmentBps = 0
	case "KV02":
		regionAdjustmentBps = 20
	case "KV03":
		regionAdjustmentBps = 40
	default:
		return 0, fmt.Errorf(
			"invalid loan region: %s", regionCode,
		)
	}

	// 3. Xác định lãi suất cơ bản cho kỳ hạn 12 tháng.
	// Bảng này lấy KV01 làm mức cơ sở.
	var baseRateBps int64

	switch {
	case monthlyIncome < 7_000_000:
		baseRateBps = 1750

	case monthlyIncome < 10_000_000:
		baseRateBps = 1690

	case monthlyIncome < 15_000_000:
		baseRateBps = 1630

	case monthlyIncome < 20_000_000:
		baseRateBps = 1570

	case monthlyIncome < 30_000_000:
		baseRateBps = 1500

	default:
		baseRateBps = 1440
	}

	// 4. Điều chỉnh lãi suất theo kỳ hạn vay.
	var termAdjustmentBps int64

	switch termMonths {
	case 6:
		termAdjustmentBps = -30

	case 12:
		termAdjustmentBps = 0

	case 24:
		termAdjustmentBps = 20

	case 36:
		termAdjustmentBps = 40

	case 48:
		termAdjustmentBps = 60

	case 60:
		termAdjustmentBps = 80

	default:
		return 0, ErrUnsupportedTerm
	}

	// 5. Lãi suất năm cuối cùng.
	annualRateBps := baseRateBps +
		regionAdjustmentBps +
		termAdjustmentBps

	return annualRateBps, nil
}
