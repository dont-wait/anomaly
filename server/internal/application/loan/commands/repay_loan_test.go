package commands

import (
	"context"
	"errors"
	"testing"
	"time"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/dont-wait/anomaly/internal/timeutil"
)

type repaymentLoanRepoStub struct {
	app *loandomain.Application
}

func (s *repaymentLoanRepoStub) FindByID(
	_ context.Context,
	id string,
) (*loandomain.Application, error) {
	if s.app == nil || s.app.Id != id {
		return nil, nil
	}
	return s.app, nil
}

type repaymentBankStub struct {
	payments []journal.LoanInstallmentPayment
	now      time.Time
	calls    int
	err      error
}

func (s *repaymentBankStub) ListLoanPayments(
	_ context.Context,
	loanID string,
) ([]journal.LoanInstallmentPayment, error) {
	result := make([]journal.LoanInstallmentPayment, 0)

	for _, p := range s.payments {
		if p.LoanID == loanID {
			result = append(result, p)
		}
	}

	return result, nil
}

func (s *repaymentBankStub) DebitLoanInstallment(
	_ context.Context,
	p journal.LoanInstallmentPayment,
) (journal.LoanInstallmentPayment, error) {
	s.calls++

	if s.err != nil {
		return journal.LoanInstallmentPayment{}, s.err
	}

	p.TransactionID = journal.LoanInstallmentTransactionID(
		p.LoanID,
		p.InstallmentNo,
	)
	p.PaymentID = journal.EventID(
		p.TransactionID,
		"payment",
	)
	p.PaidAt = s.now

	if err := p.Validate(); err != nil {
		return journal.LoanInstallmentPayment{}, err
	}

	s.payments = append(s.payments, p)

	return p, nil
}

func newLoanRepaymentCommandFixture(
	t *testing.T,
) (*loandomain.Application, *repaymentBankStub) {
	t.Helper()

	app, err := loandomain.NewDraftApplication(
		"loan-001",
		"LA-001",
		"account-001",
		"personal",
		"account-001",
		60_000_000,
		12,
	)
	if err != nil {
		t.Fatal(err)
	}

	approvedAt := time.Date(
		2026, 1, 14, 10, 0, 0, 0, time.UTC,
	)

	app.Status = loandomain.ApplicationStatusApproved
	app.ApprovedAt = &approvedAt

	err = app.MarkDisbursed(
		approvedAt.Add(24*time.Hour),
		1630,
	)
	if err != nil {
		t.Fatal(err)
	}

	first := app.RepaymentSchedule[0]
	y, m, d := first.DueDate.UTC().Date()

	bank := &repaymentBankStub{
		now: time.Date(
			y, m, d, 9, 0, 0, 0,
			timeutil.VietnamLocation,
		),
	}

	return app, bank
}

func repaymentCommand() RepayLoanCommand {
	return RepayLoanCommand{
		LoanID:          "loan-001",
		CustomerID:      "account-001",
		SourceAccountID: "account-001",
		InstallmentNo:   1,
	}
}

func TestRepayLoanSuccessAndRetry(t *testing.T) {
	app, bank := newLoanRepaymentCommandFixture(t)

	handler := &RepayLoanCommandHandler{
		Repo: &repaymentLoanRepoStub{app: app},
		Bank: bank,
		Clock: func() time.Time {
			return bank.now
		},
	}

	cmd := repaymentCommand()

	result, err := handler.Handle(
		context.Background(),
		cmd,
	)
	if err != nil {
		t.Fatal(err)
	}

	if result.PaidInstallments != 1 ||
		result.Status != loandomain.LoanRepaymentActive ||
		result.Payment.Amount !=
			app.RepaymentSchedule[0].TotalPayment ||
		result.OutstandingPrincipal !=
			app.RepaymentSchedule[0].RemainingBalance {
		t.Fatal("incorrect loan repayment result")
	}

	if bank.calls != 1 {
		t.Fatal("expected exactly one bank debit")
	}

	// Retry khong duoc tru tien lan hai.
	retried, err := handler.Handle(
		context.Background(),
		cmd,
	)
	if err != nil {
		t.Fatal(err)
	}

	if bank.calls != 1 ||
		retried.Payment.TransactionID !=
			result.Payment.TransactionID {
		t.Fatal("retry produced duplicate repayment")
	}
}

func TestRepayLoanRejectsWrongDate(t *testing.T) {
	app, bank := newLoanRepaymentCommandFixture(t)

	// Mo phong request som hon mot ngay.
	bank.now = bank.now.Add(-24 * time.Hour)

	handler := &RepayLoanCommandHandler{
		Repo: &repaymentLoanRepoStub{app: app},
		Bank: bank,
		Clock: func() time.Time {
			return bank.now
		},
	}

	_, err := handler.Handle(
		context.Background(),
		repaymentCommand(),
	)

	if !errors.Is(err, loandomain.ErrInstallmentNotDue) {
		t.Fatalf("expected not due, got %v", err)
	}

	if bank.calls != 0 {
		t.Fatal("bank debited before due date")
	}
}

func TestRepayLoanRejectsWrongInstallment(t *testing.T) {
	app, bank := newLoanRepaymentCommandFixture(t)

	handler := &RepayLoanCommandHandler{
		Repo: &repaymentLoanRepoStub{app: app},
		Bank: bank,
		Clock: func() time.Time {
			return bank.now
		},
	}

	cmd := repaymentCommand()
	cmd.InstallmentNo = 2

	_, err := handler.Handle(
		context.Background(),
		cmd,
	)

	if !errors.Is(
		err,
		loandomain.ErrInvalidInstallmentOrder,
	) {
		t.Fatalf("expected invalid order, got %v", err)
	}

	if bank.calls != 0 {
		t.Fatal("bank debited wrong installment")
	}
}

func TestRepayLoanRejectsOtherCustomer(t *testing.T) {
	app, bank := newLoanRepaymentCommandFixture(t)

	handler := &RepayLoanCommandHandler{
		Repo: &repaymentLoanRepoStub{app: app},
		Bank: bank,
		Clock: func() time.Time {
			return bank.now
		},
	}

	cmd := repaymentCommand()
	cmd.CustomerID = "account-other"
	cmd.SourceAccountID = "account-other"

	_, err := handler.Handle(context.Background(), cmd)

	if !errors.Is(err, loandomain.ErrLoanNotFound) || bank.calls != 0 {
		t.Fatal("unauthorized loan repayment accepted")
	}
}

func TestRepayLoanBankFailure(t *testing.T) {
	app, bank := newLoanRepaymentCommandFixture(t)

	bank.err = errors.New("simulated bank failure")

	handler := &RepayLoanCommandHandler{
		Repo: &repaymentLoanRepoStub{app: app},
		Bank: bank,
		Clock: func() time.Time {
			return bank.now
		},
	}

	_, err := handler.Handle(
		context.Background(),
		repaymentCommand(),
	)

	if err == nil {
		t.Fatal("expected bank failure")
	}

	if len(bank.payments) != 0 {
		t.Fatal("failed debit recorded payment")
	}
}

func TestRepayLoanReplayNextInstallment(t *testing.T) {
	app, bank := newLoanRepaymentCommandFixture(t)

	handler := &RepayLoanCommandHandler{
		Repo: &repaymentLoanRepoStub{app: app},
		Bank: bank,
		Clock: func() time.Time {
			return bank.now
		},
	}

	if _, err := handler.Handle(
		context.Background(),
		repaymentCommand(),
	); err != nil {
		t.Fatal(err)
	}

	second := app.RepaymentSchedule[1]
	y, m, d := second.DueDate.UTC().Date()

	bank.now = time.Date(
		y, m, d, 9, 0, 0, 0,
		timeutil.VietnamLocation,
	)

	cmd := repaymentCommand()
	cmd.InstallmentNo = 2

	result, err := handler.Handle(
		context.Background(),
		cmd,
	)
	if err != nil {
		t.Fatal(err)
	}

	if result.PaidInstallments != 2 ||
		result.OutstandingPrincipal !=
			second.RemainingBalance ||
		len(bank.payments) != 2 ||
		bank.calls != 2 {
		t.Fatal("repayment replay failed")
	}
}

func TestRepayLoanDifferentOwnedSourceDelegatesToBank(t *testing.T) {
	app, bank := newLoanRepaymentCommandFixture(t)
	handler := &RepayLoanCommandHandler{
		Repo: &repaymentLoanRepoStub{app: app}, Bank: bank, Clock: func() time.Time { return bank.now },
	}
	cmd := repaymentCommand()
	cmd.SourceAccountID = "another-owned-account"
	result, err := handler.Handle(context.Background(), cmd)
	if err != nil || result.Payment.AccountID != cmd.SourceAccountID {
		t.Fatalf("Bank must validate source ownership, command must not reject by ID equality: %v", err)
	}
}
