package mongo

import (
	"testing"
	"time"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Kiem tra anh xa UUID sang ObjectID on dinh.
func TestRepaymentObjectIDIsStableAndNamespaced(
	t *testing.T,
) {
	sourceID := "8c028fb3-593f-4026-a45b-36fd3e723092"

	loanID, applicationID := loanProjectionIDs(sourceID)

	nextLoanID, nextApplicationID := loanProjectionIDs(sourceID)

	if loanID != nextLoanID ||
		applicationID != nextApplicationID {

		t.Fatal("ObjectID mapping is not stable")
	}

	if loanID == applicationID {
		t.Fatal("loan and application IDs must differ")
	}

	if loanID == repaymentObjectID(
		"loan",
		"another-loan",
	) {
		t.Fatal("different loans share an ObjectID")
	}

	if _, err := bson.ObjectIDFromHex(
		loanID.Hex(),
	); err != nil {
		t.Fatal(err)
	}
}

// Kiem tra lich tra no duoc copy dung tu EMI.
func TestProjectedInstallmentsKeepsContractEMI(
	t *testing.T,
) {
	due := time.Date(
		2026, 11, 14,
		0, 0, 0, 0,
		time.UTC,
	)

	original := []loandomain.RepaymentScheduleItem{
		{
			InstallmentNo:   1,
			DueDate:         due,
			PrincipalAmount: 900_000,
			InterestAmount:  100_000,
			TotalPayment:    1_000_000,
		},
		{
			InstallmentNo:   2,
			DueDate:         due.AddDate(0, 1, 0),
			PrincipalAmount: 910_000,
			InterestAmount:  90_000,
			TotalPayment:    1_000_000,
		},
	}

	projected := projectedInstallments(original)

	if len(projected) != len(original) {
		t.Fatal("incorrect installment count")
	}

	for i, row := range projected {
		if row["installment_no"] != original[i].InstallmentNo ||
			row["principal_due"] != original[i].PrincipalAmount ||
			row["interest_due"] != original[i].InterestAmount ||
			row["total_due"] != original[i].TotalPayment ||
			row["due_date"] != original[i].DueDate ||
			row["status"] != "upcoming" {

			t.Fatalf(
				"EMI projection mismatch: %+v",
				row,
			)
		}
	}
}

// Kiem tra lai suat %/nam khong dung float.
func TestLoanRateDecimalDoesNotUseFloat(
	t *testing.T,
) {
	got, err := loanRateDecimal(1630)
	if err != nil {
		t.Fatal(err)
	}

	if got.String() != "16.30" {
		t.Fatalf(
			"expected 16.30 percent, got %s",
			got.String(),
		)
	}
}
