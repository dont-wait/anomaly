package loan

import (
	"testing"
	"time"
)

func TestEvidenceBuildAndDisbursement(t *testing.T) {
	app, err := NewDraftApplication(
		"app1",
		"LA1",
		"owner1",
		"PERSONAL_UNSECURED_SALARY",
		"owner1",
		10_000_000,
		12,
	)
	if err != nil {
		t.Fatal(err)
	}

	e := VerifiedEvidence{
		ApplicationID:         app.Id,
		CustomerID:            app.CustomerId,
		SalaryAccountID:       app.DisbursementAccountId,
		MonthlySalaries:       [3]int64{20_000_000, 20_000_000, 20_000_000},
		RegionCode:            "KV01",
		ExistingMonthlyDebt:   1_000_000,
		InsuranceMonths:       24,
		InsuranceVerified:     true,
		RepaymentHistory:      HistoryOnTime,
		HistoryVerified:       true,
		FinancialDataVerified: true,
		DocumentsVerified:     true,
		Sources:               []string{"bank:statement:1", "insurance:ref:1", "credit:history:1"},
		VerifiedBy:            "admin1",
		VerifiedAt:            time.Now().UTC(),
	}

	input, rate, err := e.BuildAssessment(app, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}

	if input.MonthlyIncome != 20_000_000 || rate <= 0 {
		t.Fatalf("invalid pricing: income=%d rate=%d", input.MonthlyIncome, rate)
	}

	app.Status = ApplicationStatusApproved
	now := time.Now().UTC()
	app.ApprovedAt = &now

	paid := now.Add(time.Second)

	if err := app.RestoreLegacyDisbursement(paid, rate); err != nil {
		t.Fatal(err)
	}

	if app.Status != ApplicationStatusDisbursed ||
		len(app.RepaymentSchedule) != 12 ||
		app.RepaymentSchedule[11].RemainingBalance != 0 {
		t.Fatal("loan was not fully scheduled")
	}

	if err := app.RestoreLegacyDisbursement(paid, rate); err == nil {
		t.Fatal("duplicate disbursement accepted")
	}
}

func TestEvidenceRejectsForgedSalaryAccount(t *testing.T) {
	app, _ := NewDraftApplication(
		"app1", "LA1", "owner1", "pkg", "owner1",
		10_000_000, 12,
	)

	e := VerifiedEvidence{
		ApplicationID:   app.Id,
		CustomerID:      app.CustomerId,
		SalaryAccountID: "someone-else",
	}

	if err := e.ValidateFor(app); err == nil {
		t.Fatal("foreign salary account was accepted")
	}
}

func TestMarkDisbursed_RejectsDifferentPreparedRate(t *testing.T) {
	app, err := NewDraftApplication(
		"loan-prepared",
		"LA-PREPARED",
		"customer-1",
		"PERSONAL_UNSECURED_SALARY",
		"account-1",
		50_000_000,
		12,
	)
	if err != nil {
		t.Fatal(err)
	}

	approvedAt := time.Now().UTC()
	app.Status = ApplicationStatusApproved
	app.ApprovedAt = &approvedAt

	const preparedRate int64 = 1540

	preparedAt := approvedAt.Add(time.Second)

	if err := app.PrepareDisbursement(
		preparedAt,
		preparedRate,
	); err != nil {
		t.Fatal(err)
	}

	creditedAt := preparedAt.Add(time.Second)

	// Lãi suất 16% khác mức đã chốt 15,4%.
	if err := app.RestoreLegacyDisbursement(
		creditedAt,
		1600,
	); err == nil {
		t.Fatal("expected mismatched interest rate to be rejected")
	}

	// Giải ngân thất bại không được thay đổi trạng thái.
	if app.Status != ApplicationStatusApproved ||
		app.DisbursedAt != nil {
		t.Fatal("loan state changed after invalid disbursement")
	}

	// Sử dụng đúng lãi suất đã chốt.
	if err := app.RestoreLegacyDisbursement(
		creditedAt,
		preparedRate,
	); err != nil {
		t.Fatalf("valid disbursement failed: %v", err)
	}

	if app.Status != ApplicationStatusDisbursed ||
		app.AnnualRateBps != preparedRate ||
		len(app.RepaymentSchedule) != 12 {
		t.Fatal("loan disbursement terms are incorrect")
	}
}
