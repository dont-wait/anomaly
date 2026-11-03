package transaction

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/timeutil"
)

func TestLoanInstallmentPaymentJournal(t *testing.T) {
	now := time.Now().UTC()
	vn := timeutil.VietnamLocation
	y, m, d := now.In(vn).Date()

	due := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	txID := LoanInstallmentTransactionID("loan-001", 1)

	payment := LoanInstallmentPayment{
		PaymentID:                 EventID(txID, "payment"),
		TransactionID:             txID,
		LoanID:                    "loan-001",
		CustomerID:                "account-001",
		AccountID:                 "account-001",
		InstallmentNo:             1,
		DueDate:                   due,
		PaidAt:                    now,
		PrincipalAmount:           900_000,
		InterestAmount:            100_000,
		Amount:                    1_000_000,
		OutstandingPrincipalAfter: 9_100_000,
	}

	before := int64(2_000_000)
	after := int64(1_000_000)

	record := Record{
		ID: EventID(
			txID,
			LoanInstallmentPaid,
		),
		Type:          LoanInstallmentPaid,
		At:            now,
		AccountID:     payment.AccountID,
		LoanID:        payment.LoanID,
		BalanceBefore: &before,
		BalanceAfter:  &after,
		LoanPayment:   &payment,
	}

	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeRecord(
		data,
		record.ID,
		record.Type,
	)
	if err != nil {
		t.Fatal(err)
	}

	if decoded.LoanPayment == nil ||
		decoded.LoanPayment.Amount != 1_000_000 ||
		decoded.LoanPayment.InstallmentNo != 1 {
		t.Fatal("loan payment replay mismatch")
	}

	// Khong chap nhan event co so tien khong hop le.
	payment.Amount++

	if err := payment.Validate(); err == nil {
		t.Fatal("invalid loan payment accepted")
	}
}

func TestLoanInstallmentPaymentAllowsLateRejectsEarly(t *testing.T) {
	due := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	tx := LoanInstallmentTransactionID("loan", 1)
	base := LoanInstallmentPayment{
		PaymentID: EventID(tx, "payment"), TransactionID: tx, LoanID: "loan", CustomerID: "customer", AccountID: "account",
		InstallmentNo: 1, PrincipalAmount: 90, InterestAmount: 10, Amount: 100, DueDate: due,
	}
	late := base
	late.PaidAt = due.AddDate(0, 0, 5)
	if err := late.Validate(); err != nil {
		t.Fatalf("late payment must replay: %v", err)
	}
	early := base
	early.PaidAt = due.AddDate(0, 0, -5)
	if err := early.Validate(); err == nil {
		t.Fatal("early payment must be rejected")
	}
}
