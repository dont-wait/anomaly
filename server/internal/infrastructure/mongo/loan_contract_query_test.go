package mongo

import (
	"testing"
	"time"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestLoanContractProjectionValidityAndMapping(t *testing.T) {
	id := "bfe4720b-4d0f-4c4c-83dd-3487901f5047"
	_, applicationOID := loanProjectionIDs(id)
	start := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	rate, err := bson.ParseDecimal128("16.70")
	if err != nil {
		t.Fatal(err)
	}
	row := repaymentLoanRow{
		ApplicationID: applicationOID,
		LoanNo:        id, Principal: 10_000_000, OutstandingPrincipal: 5_000_000,
		InterestRate: rate, TermMonths: 2, StartDate: start,
		MaturityDate: start.AddDate(0, 2, 0), Status: "active",
		Schedule: []repaymentInstallmentRow{
			{InstallmentNo: 1, DueDate: start.AddDate(0, 1, 0), PrincipalDue: 5_000_000, InterestDue: 100_000, TotalDue: 5_100_000, Status: "paid"},
			{InstallmentNo: 2, DueDate: start.AddDate(0, 2, 0), PrincipalDue: 5_000_000, InterestDue: 50_000, TotalDue: 5_050_000, Status: "upcoming"},
		},
	}
	if !validProjectedContract(row, id) {
		t.Fatal("consistent projection rejected")
	}
	summary := contractSummary(loandomain.Application{Id: id, LoanPackageId: "PERSONAL_UNSECURED_SALARY"}, row)
	if summary.PaidInstallments != 1 || summary.NextInstallmentNo != 2 || summary.NextDueAmount != 5_050_000 ||
		summary.OutstandingPrincipal != 5_000_000 || summary.AnnualInterestRate != "16.70" {
		t.Fatalf("incorrect contract summary: %+v", summary)
	}
	row.OutstandingPrincipal = 10_000_000
	if validProjectedContract(row, id) {
		t.Fatal("inconsistent outstanding principal accepted")
	}
	row.OutstandingPrincipal = 5_000_000
	row.Schedule[1].TotalDue--
	if validProjectedContract(row, id) {
		t.Fatal("incorrect installment breakdown accepted")
	}
	row.Schedule[1].TotalDue++
	row.Status = "completed"
	row.OutstandingPrincipal = 0
	if validProjectedContract(row, id) {
		t.Fatal("completed contract with unpaid installment accepted")
	}
	row.Schedule[1].Status = "paid"
	if !validProjectedContract(row, id) {
		t.Fatal("completed contract rejected")
	}
}
