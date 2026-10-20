package commands

import (
	"context"
	"errors"
	"testing"
	"time"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
)

func TestRepayLoanFullTermAndNoExtraPayment(
	t *testing.T,
) {
	app, bank := newLoanRepaymentCommandFixture(t)

	handler := &RepayLoanCommandHandler{
		Repo: &repaymentLoanRepoStub{app: app},
		Bank: bank,
		Clock: func() time.Time {
			return bank.now
		},
	}

	var final *RepayLoanResult

	// Thanh toan tung ky dung ngay den han.
	for i, due := range app.RepaymentSchedule {
		y, m, d := due.DueDate.UTC().Date()

		bank.now = time.Date(
			y, m, d,
			10, 0, 0, 0,
			time.FixedZone("ICT", 7*3600),
		)

		cmd := repaymentCommand()
		cmd.InstallmentNo = i + 1

		result, err := handler.Handle(
			context.Background(), cmd,
		)
		if err != nil {
			t.Fatalf(
				"installment %d: %v",
				i+1, err,
			)
		}

		if result.PaidInstallments != i+1 ||
			result.OutstandingPrincipal !=
				due.RemainingBalance {
			t.Fatalf(
				"installment %d has invalid outstanding balance",
				i+1,
			)
		}

		final = result
	}

	// Sau ky cuoi, khoan vay phai tat toan.
	if final == nil ||
		final.OutstandingPrincipal != 0 ||
		final.Status !=
			loandomain.LoanRepaymentCompleted {

		t.Fatalf(
			"loan not completed: %+v",
			final,
		)
	}

	if len(bank.payments) != app.TermMonths ||
		bank.calls != app.TermMonths {
		t.Fatalf(
			"payments=%d debits=%d",
			len(bank.payments),
			bank.calls,
		)
	}

	// Retry ky cuoi: tra bien nhan cu,
	// khong tru tien lan hai.
	cmd := repaymentCommand()
	cmd.InstallmentNo = app.TermMonths

	retry, err := handler.Handle(
		context.Background(), cmd,
	)

	if err != nil ||
		retry.Payment.TransactionID !=
			final.Payment.TransactionID ||
		bank.calls != app.TermMonths {

		t.Fatalf(
			"terminal retry failed: %v",
			err,
		)
	}

	// Khong cho thu ky moi sau tat toan.
	cmd.InstallmentNo++

	_, err = handler.Handle(
		context.Background(), cmd,
	)

	if !errors.Is(
		err,
		loandomain.ErrLoanRepaymentCompleted,
	) || bank.calls != app.TermMonths {

		t.Fatalf(
			"extra installment accepted: %v",
			err,
		)
	}
}
