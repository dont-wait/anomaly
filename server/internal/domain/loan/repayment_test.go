package loan

import (
	"errors"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/timeutil"
)

// Tao mot khoan vay da giai ngan de kiem thu.
func newDisbursedRepayment(
	t *testing.T,
	termMonths int,
) *LoanRepayment {
	t.Helper()

	app, err := NewDraftApplication(
		"app-001",
		"LA-001",
		"customer-001",
		"personal",
		"account-001",
		60_000_000,
		termMonths,
	)
	if err != nil {
		t.Fatal(err)
	}

	approvedAt := time.Date(
		2026, 1, 14, 10, 0, 0, 0, time.UTC,
	)

	app.Status = ApplicationStatusApproved
	app.ApprovedAt = &approvedAt

	err = app.MarkDisbursed(
		approvedAt.Add(24*time.Hour),
		1630,
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := NewLoanRepaymentFromDisbursed(app)
	if err != nil {
		t.Fatal(err)
	}

	return result
}

// Lay thoi diem 9 gio sang dung ngay den han.
func onDueDay(
	item RepaymentScheduleItem,
) time.Time {
	y, m, d := item.DueDate.Date()

	return time.Date(
		y, m, d,
		9, 0, 0, 0,
		timeutil.VietnamLocation,
	)
}

// Test tra dung mot ky.
func TestLoanRepaymentPayExactlyOneInstallment(
	t *testing.T,
) {
	loan := newDisbursedRepayment(t, 12)
	first := loan.Schedule[0]

	paid, err := loan.RecordInstallmentPayment(
		1,
		onDueDay(first),
	)
	if err != nil {
		t.Fatal(err)
	}

	if paid.TotalPayment != first.TotalPayment ||
		paid.PrincipalAmount != first.PrincipalAmount ||
		paid.InterestAmount != first.InterestAmount ||
		loan.OutstandingPrincipal != first.RemainingBalance ||
		loan.PaidInstallments != 1 ||
		loan.Status != LoanRepaymentActive {

		t.Fatal("incorrect installment payment result")
	}

	status, err := loan.InstallmentStatus(1)
	if err != nil || status != LoanInstallmentPaid {
		t.Fatalf(
			"first installment status = %q, error %v",
			status,
			err,
		)
	}
}

// Test tra het cac ky han duoc ho tro.
func TestLoanRepaymentAllSupportedTerms(
	t *testing.T,
) {
	for _, term := range []int{
		6, 12, 24, 36, 48, 60,
	} {
		t.Run(
			time.Duration(term).String(),
			func(t *testing.T) {
				loan := newDisbursedRepayment(t, term)

				var principalPaid int64

				for i, due := range loan.Schedule {
					paid, err := loan.RecordInstallmentPayment(
						i+1,
						onDueDay(due),
					)
					if err != nil {
						t.Fatalf(
							"installment %d: %v",
							i+1,
							err,
						)
					}

					principalPaid += paid.PrincipalAmount
				}

				if principalPaid != loan.Principal ||
					loan.OutstandingPrincipal != 0 ||
					loan.PaidInstallments != term ||
					loan.Status != LoanRepaymentCompleted {

					t.Fatal(
						"loan is not completed after all installments",
					)
				}

				_, err := loan.DueInstallment(
					onDueDay(loan.Schedule[term-1]),
				)

				if !errors.Is(
					err,
					ErrLoanRepaymentCompleted,
				) {
					t.Fatalf(
						"expected loan completed, got %v",
						err,
					)
				}
			},
		)
	}
}

// Test khong cho tra truoc hoac sau ngay den han.
func TestLoanRepaymentRejectsEarlyAndLatePayments(
	t *testing.T,
) {
	loan := newDisbursedRepayment(t, 12)

	first := onDueDay(loan.Schedule[0])

	for _, at := range []time.Time{
		first.Add(-24 * time.Hour),
		first.Add(24 * time.Hour),
	} {
		_, err := loan.RecordInstallmentPayment(1, at)

		if !errors.Is(err, ErrInstallmentNotDue) {
			t.Fatalf(
				"expected wrong day rejection, got %v",
				err,
			)
		}
	}

	if loan.PaidInstallments != 0 ||
		loan.OutstandingPrincipal != loan.Principal {

		t.Fatal("invalid attempt modified loan")
	}
}

// Test khong cho bo qua ky va thanh toan trung.
func TestLoanRepaymentRejectsSkipAndDuplicate(
	t *testing.T,
) {
	loan := newDisbursedRepayment(t, 12)

	_, err := loan.RecordInstallmentPayment(
		2,
		onDueDay(loan.Schedule[1]),
	)

	if !errors.Is(err, ErrInvalidInstallmentOrder) {
		t.Fatalf(
			"skip installment accepted: %v",
			err,
		)
	}

	first := onDueDay(loan.Schedule[0])

	if _, err := loan.RecordInstallmentPayment(
		1, first,
	); err != nil {
		t.Fatal(err)
	}

	_, err = loan.RecordInstallmentPayment(1, first)

	if !errors.Is(err, ErrInstallmentAlreadyPaid) {
		t.Fatalf(
			"duplicate installment accepted: %v",
			err,
		)
	}

	if loan.PaidInstallments != 1 {
		t.Fatal("duplicate changed paid installments")
	}
}

// Test ho so chua giai ngan khong duoc tra no.
func TestLoanRepaymentSourceRequiresDisbursement(
	t *testing.T,
) {
	if _, err := NewLoanRepaymentFromDisbursed(
		nil,
	); !errors.Is(err, ErrLoanNotDisbursed) {
		t.Fatal(err)
	}

	app, err := NewDraftApplication(
		"id", "no", "customer", "pkg",
		"acc", 10_000_000, 12,
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := NewLoanRepaymentFromDisbursed(
		app,
	); !errors.Is(err, ErrLoanNotDisbursed) {
		t.Fatal(err)
	}
}

// Test phat hien lich tra no bi sai.
func TestLoanRepaymentRejectsBrokenSchedule(
	t *testing.T,
) {
	app, err := NewDraftApplication(
		"id", "no", "customer", "pkg",
		"acc", 10_000_000, 12,
	)
	if err != nil {
		t.Fatal(err)
	}

	approvedAt := time.Date(
		2026, 1, 14, 10, 0, 0, 0, time.UTC,
	)

	app.Status = ApplicationStatusApproved
	app.ApprovedAt = &approvedAt

	if err := app.MarkDisbursed(
		approvedAt.Add(24*time.Hour),
		1630,
	); err != nil {
		t.Fatal(err)
	}

	app.RepaymentSchedule[0].TotalPayment++

	_, err = NewLoanRepaymentFromDisbursed(app)

	if !errors.Is(err, ErrInvalidRepaymentState) {
		t.Fatalf(
			"broken schedule accepted: %v",
			err,
		)
	}
}

// Test lich tra no duoc sao chep rieng,
// khong bi thay doi theo Application ban dau.
func TestLoanRepaymentCopiesSchedule(
	t *testing.T,
) {
	app, err := NewDraftApplication(
		"id", "no", "customer", "pkg",
		"acc", 10_000_000, 12,
	)
	if err != nil {
		t.Fatal(err)
	}

	approvedAt := time.Date(
		2026, 1, 14, 10, 0, 0, 0, time.UTC,
	)

	app.Status = ApplicationStatusApproved
	app.ApprovedAt = &approvedAt

	if err := app.MarkDisbursed(
		approvedAt.Add(24*time.Hour),
		1630,
	); err != nil {
		t.Fatal(err)
	}

	loan, err := NewLoanRepaymentFromDisbursed(app)
	if err != nil {
		t.Fatal(err)
	}

	original := loan.Schedule[0].PrincipalAmount

	app.RepaymentSchedule[0].PrincipalAmount = -1

	if loan.Schedule[0].PrincipalAmount != original {
		t.Fatal(
			"repayment schedule shares source application memory",
		)
	}
}
