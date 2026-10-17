package commands

import (
	"context"
	"strings"
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

	// Evidence hợp lệ về dữ liệu thẩm định.
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

	// Thẩm định hồ sơ bằng Evidence hợp lệ.
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
			"expected auto approval, got %s",
			assessment.Decision,
		)
	}

	// Giả lập chứng từ chưa được xác minh đầy đủ.
	// Các dữ liệu thẩm định khác vẫn giữ nguyên.
	e.DocumentsVerified = false

	repo := &addLoanDocumentRepoStub{
		app:      app,
		revision: 10,
	}

	// Bank có thể giải ngân thành công.
	// Không dùng lỗi "bank offline" để che lỗi nghiệp vụ.
	bank := &disburseBankStub{
		at: time.Now().UTC().Add(time.Second),
	}

	handler := &DisburseLoanCommandHandler{
		Repo:       repo,
		Authorizer: &manualLoanAuthorizerStub{},
		Evidence:   disburseEvidenceStub{e: e},
		Bank:       bank,
	}

	_, err = handler.Handle(
		context.Background(),
		DisburseLoanCommand{
			ApplicationID: app.Id,
			ActorID:       "STAFF001",
		},
	)

	// Phải từ chối vì Evidence chưa xác minh.
	if err == nil {
		t.Fatal("expected invalid evidence to be rejected")
	}

	if !strings.Contains(err.Error(), "not fully verified") {
		t.Fatalf("unexpected error: %v", err)
	}

	// Tuyệt đối không được gọi Banking.
	if bank.calls != 0 {
		t.Fatalf(
			"bank was called %d times for invalid evidence",
			bank.calls,
		)
	}

	// Không được ghi bất kỳ Loan Event nào.
	if repo.appendCalls != 0 {
		t.Fatalf(
			"unexpected loan event append: %d",
			repo.appendCalls,
		)
	}

	// Hồ sơ vẫn phải giữ trạng thái approved.
	if app.Status != domain.ApplicationStatusApproved {
		t.Fatal("loan status changed after invalid evidence")
	}
}
