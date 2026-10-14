package loan

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Dữ liệu do nhân viên xác nhận trong hệ thống mô phỏng.
// Không phải dữ liệu tự động lấy từ CIC, BHXH hoặc ngân hàng.
type VerifiedEvidence struct {
	ApplicationID         string                 `json:"applicationId" bson:"application_id"`
	CustomerID            string                 `json:"customerId" bson:"customer_id"`
	SalaryAccountID       string                 `json:"salaryAccountId" bson:"salary_account_id"`
	MonthlySalaries       [3]int64               `json:"monthlySalaries" bson:"monthly_salaries"`
	RegionCode            string                 `json:"regionCode" bson:"region_code"`
	ExistingMonthlyDebt   int64                  `json:"existingMonthlyDebt" bson:"existing_monthly_debt"`
	InsuranceMonths       int                    `json:"insuranceMonths" bson:"insurance_months"`
	InsuranceVerified     bool                   `json:"insuranceVerified" bson:"insurance_verified"`
	RepaymentHistory      RepaymentHistoryStatus `json:"repaymentHistory" bson:"repayment_history"`
	HistoryVerified       bool                   `json:"historyVerified" bson:"history_verified"`
	FinancialDataVerified bool                   `json:"financialDataVerified" bson:"financial_data_verified"`
	DocumentsVerified     bool                   `json:"documentsVerified" bson:"documents_verified"`
	Sources               []string               `json:"sources" bson:"sources"`
	VerifiedBy            string                 `json:"verifiedBy" bson:"verified_by"`
	VerifiedAt            time.Time              `json:"verifiedAt" bson:"verified_at"`
}

func (e VerifiedEvidence) AverageMonthlyIncome() (int64, error) {
	var sum int64

	for _, salary := range e.MonthlySalaries {
		if salary <= 0 || salary > math.MaxInt64-sum {
			return 0, fmt.Errorf("invalid three-month salary evidence")
		}
		sum += salary
	}

	return sum / 3, nil
}

func (e VerifiedEvidence) ValidateFor(a *Application) error {
	if a == nil ||
		e.ApplicationID != a.Id ||
		e.CustomerID != a.CustomerId ||
		e.SalaryAccountID != a.DisbursementAccountId {
		return fmt.Errorf(
			"evidence does not match loan application and salary account",
		)
	}

	if strings.TrimSpace(e.VerifiedBy) == "" ||
		e.VerifiedAt.IsZero() ||
		len(e.Sources) < 3 {
		return fmt.Errorf(
			"evidence must include staff identity, timestamp and source references",
		)
	}

	for _, source := range e.Sources {
		if strings.TrimSpace(source) == "" {
			return fmt.Errorf("empty evidence source reference")
		}
	}

	if _, err := e.AverageMonthlyIncome(); err != nil {
		return err
	}

	if e.ExistingMonthlyDebt < 0 || e.InsuranceMonths < 0 {
		return fmt.Errorf("negative financial or insurance evidence")
	}

	if _, err := CalculateAnnualLoanRateBps(
		1,
		e.RegionCode,
		a.TermMonths,
	); err != nil {
		return err
	}

	switch e.RepaymentHistory {
	case HistoryOnTime,
		HistoryNever,
		HistoryLate,
		HistoryOverdue,
		HistoryUnknown:
	default:
		return fmt.Errorf("invalid repayment history")
	}

	return nil
}

func (e VerifiedEvidence) BuildAssessment(
	a *Application,
	now time.Time,
) (LoanAssessmentInput, int64, error) {
	if err := e.ValidateFor(a); err != nil {
		return LoanAssessmentInput{}, 0, err
	}

	income, _ := e.AverageMonthlyIncome()

	rate, err := CalculateAnnualLoanRateBps(
		income,
		e.RegionCode,
		a.TermMonths,
	)
	if err != nil {
		return LoanAssessmentInput{}, 0, err
	}

	affordability, err := EvaluateLoanAffordability(
		income,
		e.ExistingMonthlyDebt,
		a.RequestedAmount,
		rate,
		a.TermMonths,
		now,
	)
	if err != nil {
		return LoanAssessmentInput{}, 0, err
	}

	input := LoanAssessmentInput{
		MonthlyIncome:         income,
		Affordability:         affordability,
		InsuranceMonths:       e.InsuranceMonths,
		InsuranceVerified:     e.InsuranceVerified,
		RepaymentHistory:      e.RepaymentHistory,
		HistoryVerified:       e.HistoryVerified,
		FinancialDataVerified: e.FinancialDataVerified,
		DocumentsVerified:     e.DocumentsVerified,
	}

	return input, rate, nil
}
