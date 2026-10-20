package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
)

// LoanRepaymentReader doc ho so vay tu EventStoreDB.
type LoanRepaymentReader interface {
	FindByID(
		ctx context.Context,
		loanID string,
	) (*loandomain.Application, error)
}

// LoanRepaymentBank xu ly tien trong Banking Stream.
type LoanRepaymentBank interface {
	ListLoanPayments(
		ctx context.Context,
		loanID string,
	) ([]journal.LoanInstallmentPayment, error)

	DebitLoanInstallment(
		ctx context.Context,
		p journal.LoanInstallmentPayment,
	) (journal.LoanInstallmentPayment, error)
}

type RepayLoanCommand struct {
	LoanID          string
	CustomerID      string
	SourceAccountID string
	InstallmentNo   int
}

type RepayLoanResult struct {
	Payment              journal.LoanInstallmentPayment
	OutstandingPrincipal int64
	PaidInstallments     int
	Status               loandomain.LoanRepaymentStatus
}

type RepayLoanCommandHandler struct {
	Repo  LoanRepaymentReader
	Bank  LoanRepaymentBank
	Clock func() time.Time
}

func (h *RepayLoanCommandHandler) now() time.Time {
	if h.Clock != nil {
		return h.Clock().UTC()
	}
	return time.Now().UTC()
}

func (h *RepayLoanCommandHandler) Handle(
	ctx context.Context,
	cmd RepayLoanCommand,
) (*RepayLoanResult, error) {
	if h == nil || h.Repo == nil || h.Bank == nil {
		return nil, fmt.Errorf("repayment services unavailable")
	}

	cmd.LoanID = strings.TrimSpace(cmd.LoanID)
	cmd.CustomerID = strings.TrimSpace(cmd.CustomerID)
	cmd.SourceAccountID = strings.TrimSpace(cmd.SourceAccountID)

	if cmd.LoanID == "" ||
		cmd.CustomerID == "" ||
		cmd.SourceAccountID == "" ||
		cmd.InstallmentNo <= 0 {
		return nil, fmt.Errorf("invalid repayment request")
	}

	// 1. Doc ho so vay tu Loan Stream.
	app, err := h.Repo.FindByID(ctx, cmd.LoanID)
	if err != nil {
		return nil, err
	}

	if app == nil || app.Id != cmd.LoanID {
		return nil, fmt.Errorf("loan not found")
	}

	if app.CustomerId != cmd.CustomerID {
		return nil, fmt.Errorf("loan not found")
	}

	// 2. Khoan vay phai da giai ngan.
	repayment, err := loandomain.NewLoanRepaymentFromDisbursed(
		app,
	)
	if err != nil {
		return nil, err
	}

	// 3. Doc lich su thanh toan tu Banking Stream.
	// Khong doc MongoDB, vi projection co the bi tre.
	history, err := h.Bank.ListLoanPayments(ctx, cmd.LoanID)
	if err != nil {
		return nil, err
	}

	if len(history) > repayment.TermMonths {
		return nil, fmt.Errorf("invalid repayment history length")
	}

	// 4. Replay cac ky da thanh toan.
	for i, old := range history {
		expected := repayment.Schedule[i]

		if old.LoanID != cmd.LoanID ||
			old.CustomerID != cmd.CustomerID ||
			old.InstallmentNo != i+1 ||
			old.PrincipalAmount != expected.PrincipalAmount ||
			old.InterestAmount != expected.InterestAmount ||
			old.Amount != expected.TotalPayment ||
			old.OutstandingPrincipalAfter !=
				expected.RemainingBalance ||
			!old.DueDate.Equal(expected.DueDate) {
			return nil, fmt.Errorf(
				"repayment history mismatch at installment %d",
				i+1,
			)
		}

		if err := old.Validate(); err != nil {
			return nil, fmt.Errorf(
				"invalid repayment history: %w",
				err,
			)
		}

		_, err := repayment.RecordInstallmentPayment(
			old.InstallmentNo,
			old.PaidAt,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"cannot replay installment %d: %w",
				i+1,
				err,
			)
		}
	}

	// 5. Request trung: tra ve ket qua da luu.
	// Khong tru tien lan hai.
	if cmd.InstallmentNo <= repayment.PaidInstallments {
		old := history[cmd.InstallmentNo-1]

		if old.AccountID != cmd.SourceAccountID {
			return nil, fmt.Errorf(
				"repayment account conflict",
			)
		}

		return &RepayLoanResult{
			Payment:              old,
			OutstandingPrincipal: repayment.OutstandingPrincipal,
			PaidInstallments:     repayment.PaidInstallments,
			Status:               repayment.Status,
		}, nil
	}

	// 6. Phai tra theo dung thu tu ky.
	if cmd.InstallmentNo != repayment.PaidInstallments+1 {
		return nil, loandomain.ErrInvalidInstallmentOrder
	}

	// 7. Kiem tra dung ngay den han.
	due, err := repayment.DueInstallment(h.now())
	if err != nil {
		return nil, err
	}

	// 8. So tien lay tu lich EMI da chot.
	// Tuyet doi khong lay lai bang lai suat hien tai.
	request := journal.LoanInstallmentPayment{
		LoanID:                    repayment.LoanID,
		CustomerID:                repayment.CustomerID,
		AccountID:                 cmd.SourceAccountID,
		InstallmentNo:             due.InstallmentNo,
		DueDate:                   due.DueDate,
		PrincipalAmount:           due.PrincipalAmount,
		InterestAmount:            due.InterestAmount,
		Amount:                    due.TotalPayment,
		OutstandingPrincipalAfter: due.RemainingBalance,
	}

	// 9. Ghi no va event trong mot Banking Stream append.
	paid, err := h.Bank.DebitLoanInstallment(
		ctx,
		request,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"debit loan installment failed: %w",
			err,
		)
	}

	// 10. Kiem tra ket qua Banking tra ve.
	// Neu ket qua bat thuong, dung ngay, khong thu lai.
	if paid.LoanID != cmd.LoanID ||
		paid.CustomerID != cmd.CustomerID ||
		paid.AccountID != cmd.SourceAccountID ||
		paid.InstallmentNo != due.InstallmentNo ||
		paid.Amount != due.TotalPayment ||
		paid.PrincipalAmount != due.PrincipalAmount ||
		paid.InterestAmount != due.InterestAmount ||
		paid.OutstandingPrincipalAfter != due.RemainingBalance ||
		!paid.DueDate.Equal(due.DueDate) {
		return nil, fmt.Errorf(
			"bank repayment receipt does not match schedule",
		)
	}

	if err := paid.Validate(); err != nil {
		return nil, err
	}

	// 11. Cap nhat Domain trong memory.
	_, err = repayment.RecordInstallmentPayment(
		paid.InstallmentNo,
		paid.PaidAt,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"bank payment recorded but domain replay failed: %w",
			err,
		)
	}

	return &RepayLoanResult{
		Payment:              paid,
		OutstandingPrincipal: repayment.OutstandingPrincipal,
		PaidInstallments:     repayment.PaidInstallments,
		Status:               repayment.Status,
	}, nil
}
