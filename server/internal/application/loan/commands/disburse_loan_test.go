package commands

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/dont-wait/anomaly/internal/domain/loan"
)

type disburseEvidenceStub struct {
	e   domain.VerifiedEvidence
	err error
}

func (s disburseEvidenceStub) Find(
	_ context.Context,
	_ string,
) (domain.VerifiedEvidence, error) {
	return s.e, s.err
}

type disburseBankStub struct {
	calls int
	err   error
	at    time.Time
}

func (s *disburseBankStub) CreditLoanDisbursement(
	_ context.Context,
	_, _ string,
	_ int64,
) (time.Time, error) {
	s.calls++
	return s.at, s.err
}

func TestDisburseLoan_SuccessAndIdempotency(t *testing.T) {
	app := newLoanForAssessmentTest(t)

	e := domain.VerifiedEvidence{
		ApplicationID:   app.Id,
		CustomerID:      app.CustomerId,
		SalaryAccountID: app.DisbursementAccountId,
		MonthlySalaries: [3]int64{
			100_000_000,
			100_000_000,
			100_000_000,
		},
		RegionCode:            "KV01",
		ExistingMonthlyDebt:   0,
		InsuranceMonths:       24,
		InsuranceVerified:     true,
		RepaymentHistory:      domain.HistoryOnTime,
		HistoryVerified:       true,
		FinancialDataVerified: true,
		DocumentsVerified:     true,
		Sources: []string{
			"bank-ref",
			"insurance-ref",
			"credit-ref",
		},
		VerifiedBy: "STAFF001",
		VerifiedAt: time.Now().UTC(),
	}

	input, _, err := e.BuildAssessment(
		app,
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}

	assessment, err := app.ApplyAssessment(input)
	if err != nil {
		t.Fatal(err)
	}

	if assessment.Decision != domain.AssessmentAutoEligible {
		t.Fatalf(
			"expected approval, got %s",
			assessment.Decision,
		)
	}

	repo := &addLoanDocumentRepoStub{
		app:      app,
		revision: 10,
	}

	bank := &disburseBankStub{
		at: time.Now().UTC().Add(time.Second),
	}

	handler := &DisburseLoanCommandHandler{
		Repo:       repo,
		Authorizer: &manualLoanAuthorizerStub{},
		Evidence:   disburseEvidenceStub{e: e},
		Bank:       bank,
	}

	result, err := handler.Handle(
		context.Background(),
		DisburseLoanCommand{
			ApplicationID: app.Id,
			ActorID:       "STAFF001",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if result.Status != domain.ApplicationStatusDisbursed ||
		repo.savedType != "LoanDisbursed" ||
		bank.calls != 1 {
		t.Fatal("loan disbursement not persisted")
	}

	if app.Status != domain.ApplicationStatusApproved {
		t.Fatal("source aggregate changed")
	}

	// Giả lập đọc lại hồ sơ đã giải ngân.
	repo.app = result

	_, err = handler.Handle(
		context.Background(),
		DisburseLoanCommand{
			ApplicationID: app.Id,
			ActorID:       "STAFF001",
		},
	)

	if err != nil || bank.calls != 1 {
		t.Fatal("idempotent retry credited twice")
	}
}

func TestDisburseLoan_InvalidEvidenceDoesNotAppend(t *testing.T) {
	app := newLoanForAssessmentTest(t)

	input := validAssessmentCommandInput()
	_, _ = app.ApplyAssessment(input)

	repo := &addLoanDocumentRepoStub{
		app:      app,
		revision: 10,
	}

	handler := &DisburseLoanCommandHandler{
		Repo:       repo,
		Authorizer: &manualLoanAuthorizerStub{},
		Evidence: disburseEvidenceStub{
			e: domain.VerifiedEvidence{},
		},
		Bank: &disburseBankStub{
			err: errors.New("bank offline"),
		},
	}

	_, err := handler.Handle(
		context.Background(),
		DisburseLoanCommand{
			ApplicationID: app.Id,
			ActorID:       "STAFF001",
		},
	)

	if err == nil || repo.appendCalls != 0 {
		t.Fatal("invalid disbursement must not append")
	}
}
