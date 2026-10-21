package queries

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	domain "github.com/dont-wait/anomaly/internal/domain/loan"
)

var (
	ErrLoanPackageUnavailable = errors.New("loan package is not available")
	ErrInvalidLoanQuoteInput  = errors.New("invalid loan quote input")
)

type PreApplicationQuoteInput struct {
	LoanPackageID       string `json:"loanPackageId"`
	RequestedAmount     int64  `json:"requestedAmount"`
	TermMonths          int    `json:"termMonths"`
	MonthlyIncome       int64  `json:"monthlyIncome"`
	RegionCode          string `json:"regionCode"`
	ExistingMonthlyDebt int64  `json:"existingMonthlyDebt"`
}

type PreApplicationQuoteResult struct {
	LoanPackageID           string                         `json:"loanPackageId"`
	RequestedAmount         int64                          `json:"requestedAmount"`
	TermMonths              int                            `json:"termMonths"`
	AnnualRateBps           int64                          `json:"annualRateBps"`
	MonthlyIncome           int64                          `json:"monthlyIncome"`
	AssumedDisbursementDate time.Time                      `json:"assumedDisbursementDate"`
	EstimatedMonthlyPayment int64                          `json:"estimatedMonthlyPayment"`
	TotalInterest           int64                          `json:"totalInterest"`
	TotalRepayment          int64                          `json:"totalRepayment"`
	Affordability           *domain.LoanAffordability      `json:"affordability"`
	ProvisionalSchedule     []domain.RepaymentScheduleItem `json:"provisionalSchedule"`
	Official                bool                           `json:"official"`
}

type PreApplicationQuoteQueryService struct {
	Reader LoanPackageReader
	Now    func() time.Time
}

func NewPreApplicationQuoteQueryService(reader LoanPackageReader) *PreApplicationQuoteQueryService {
	return &PreApplicationQuoteQueryService{Reader: reader, Now: time.Now}
}

// Quote chỉ ước tính theo dữ liệu khách hàng nhập; không ghi sự kiện,
// không xác minh thu nhập và không cam kết phê duyệt hay lãi suất hợp đồng.
func (s *PreApplicationQuoteQueryService) Quote(ctx context.Context, in PreApplicationQuoteInput) (*PreApplicationQuoteResult, error) {
	if s == nil || s.Reader == nil {
		return nil, errors.New("loan package reader is not configured")
	}
	if in.LoanPackageID == "" || in.RequestedAmount <= 0 || in.MonthlyIncome <= 0 || in.ExistingMonthlyDebt < 0 {
		return nil, fmt.Errorf("%w: amount, income, debt and package must be valid", ErrInvalidLoanQuoteInput)
	}

	packages, err := s.Reader.ListActiveLoanPackages(ctx)
	if err != nil {
		return nil, err
	}

	var selected *LoanPackage
	for i := range packages {
		if packages[i].PackageCode == in.LoanPackageID {
			selected = &packages[i]
			break
		}
	}
	if selected == nil {
		return nil, ErrLoanPackageUnavailable
	}
	if selected.MinAmount <= 0 || selected.MaxAmount < selected.MinAmount || len(selected.TermMonths) == 0 {
		return nil, errors.New("loan package configuration is invalid")
	}
	if in.RequestedAmount < selected.MinAmount || in.RequestedAmount > selected.MaxAmount {
		return nil, fmt.Errorf("%w: amount is outside the selected package range", ErrInvalidLoanQuoteInput)
	}

	validTerm := false
	for _, months := range selected.TermMonths {
		if months == in.TermMonths {
			validTerm = true
			break
		}
	}
	if !validTerm {
		return nil, fmt.Errorf("%w: unsupported loan term", ErrInvalidLoanQuoteInput)
	}
	rate, err := domain.CalculateAnnualLoanRateBps(in.MonthlyIncome, in.RegionCode, in.TermMonths)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidLoanQuoteInput, err)
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}

	schedule, err := domain.CalculateRepaymentSchedule(in.RequestedAmount, rate, in.TermMonths, now)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidLoanQuoteInput, err)
	}
	affordability, err := domain.EvaluateLoanAffordability(
		in.MonthlyIncome, in.ExistingMonthlyDebt, in.RequestedAmount, rate, in.TermMonths, now,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidLoanQuoteInput, err)
	}

	var interest, total int64
	for _, installment := range schedule {
		if installment.InterestAmount < 0 || installment.TotalPayment < 0 ||
			interest > math.MaxInt64-installment.InterestAmount ||
			total > math.MaxInt64-installment.TotalPayment {
			return nil, fmt.Errorf("%w: calculated total exceeds int64", ErrInvalidLoanQuoteInput)
		}
		interest += installment.InterestAmount
		total += installment.TotalPayment
	}

	return &PreApplicationQuoteResult{
		LoanPackageID:           in.LoanPackageID,
		RequestedAmount:         in.RequestedAmount,
		TermMonths:              in.TermMonths,
		AnnualRateBps:           rate,
		MonthlyIncome:           in.MonthlyIncome,
		AssumedDisbursementDate: now,
		EstimatedMonthlyPayment: affordability.EstimatedMonthlyPayment,
		TotalInterest:           interest,
		TotalRepayment:          total,
		Affordability:           affordability,
		ProvisionalSchedule:     schedule,
		Official:                false,
	}, nil
}
