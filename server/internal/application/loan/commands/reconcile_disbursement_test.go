package commands

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	domain "github.com/dont-wait/anomaly/internal/domain/loan"
)

// Repository giả lập việc ghi LoanDisbursed.
// Không kết nối EventStoreDB thật.
type reconcileRepoStub struct {
	LoanDocumentRepository

	app         *domain.Application
	revision    uint64
	appendCalls int
	failNext    bool
}

func (s *reconcileRepoStub) FindByIDWithRevision(
	_ context.Context,
	id string,
) (*domain.Application, uint64, error) {
	if s.app == nil || s.app.Id != id {
		return nil, 0, nil
	}

	return s.app, s.revision, nil
}

func (s *reconcileRepoStub) Append(
	_ context.Context,
	id string,
	revision uint64,
	eventType string,
	payload any,
) error {
	s.appendCalls++

	if s.failNext {
		s.failNext = false
		return errors.New("simulated append failure")
	}

	if s.app == nil || s.app.Id != id {
		return errors.New("loan not found")
	}

	if revision != s.revision {
		return errors.New("revision conflict")
	}

	if eventType != "LoanDisbursed" {
		return errors.New("unexpected event type")
	}

	// Giả lập EventStoreDB lưu event thành công,
	// sau đó replay để có trạng thái mới.
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var event struct {
		DisbursedAt   time.Time `json:"disbursedAt"`
		AnnualRateBps int64     `json:"annualRateBps"`
	}

	if err := json.Unmarshal(data, &event); err != nil {
		return err
	}

	next := *s.app

	if err := next.MarkDisbursed(
		event.DisbursedAt,
		event.AnnualRateBps,
	); err != nil {
		return err
	}

	s.app = &next
	s.revision++

	return nil
}

func prepareReconcileLoanTest(
	t *testing.T,
) (*domain.Application, domain.VerifiedEvidence) {
	t.Helper()

	app := newLoanForAssessmentTest(t)

	evidence := domain.VerifiedEvidence{
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

	input, _, err := evidence.BuildAssessment(
		app,
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := app.ApplyAssessment(input); err != nil {
		t.Fatal(err)
	}

	if app.Status != domain.ApplicationStatusApproved {
		t.Fatalf("expected approved, got %s", app.Status)
	}

	return app, evidence
}

func TestReconcileCreditedLoan_SuccessAndIdempotency(t *testing.T) {
	app, evidence := prepareReconcileLoanTest(t)

	repo := &reconcileRepoStub{
		app:      app,
		revision: 10,
	}

	handler := &ReconcileCreditedLoanCommandHandler{
		Repo:     repo,
		Evidence: disburseEvidenceStub{e: evidence},
	}

	cmd := ReconcileCreditedLoanCommand{
		LoanID:     app.Id,
		AccountID:  app.DisbursementAccountId,
		Amount:     app.RequestedAmount,
		CreditedAt: time.Now().UTC().Add(time.Second),
	}

	// Tiền đã cộng ở Banking Stream.
	// Reconciliation chỉ hoàn tất Loan Stream.
	if err := handler.Handle(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}

	if repo.app.Status != domain.ApplicationStatusDisbursed {
		t.Fatal("loan was not finalized")
	}

	if repo.appendCalls != 1 {
		t.Fatalf("expected 1 append, got %d", repo.appendCalls)
	}

	// Chạy lại sau khi đã hoàn tất.
	if err := handler.Handle(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}

	if repo.appendCalls != 1 {
		t.Fatal("retry appended LoanDisbursed twice")
	}
}

func TestReconcileCreditedLoan_RetryAfterAppendFailure(t *testing.T) {
	app, evidence := prepareReconcileLoanTest(t)

	repo := &reconcileRepoStub{
		app:      app,
		revision: 10,
		failNext: true,
	}

	handler := &ReconcileCreditedLoanCommandHandler{
		Repo:     repo,
		Evidence: disburseEvidenceStub{e: evidence},
	}

	cmd := ReconcileCreditedLoanCommand{
		LoanID:     app.Id,
		AccountID:  app.DisbursementAccountId,
		Amount:     app.RequestedAmount,
		CreditedAt: time.Now().UTC().Add(time.Second),
	}

	// Lần đầu giả lập EventStoreDB ghi thất bại.
	err := handler.Handle(context.Background(), cmd)

	if err == nil {
		t.Fatal("expected append failure")
	}

	if repo.app.Status != domain.ApplicationStatusApproved {
		t.Fatal("loan should remain approved after failed append")
	}

	// Lần sau phục hồi thành công.
	if err := handler.Handle(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}

	if repo.app.Status != domain.ApplicationStatusDisbursed {
		t.Fatal("retry did not finalize loan")
	}

	if repo.appendCalls != 2 {
		t.Fatalf("expected 2 append attempts, got %d", repo.appendCalls)
	}
}

func TestReconcileCreditedLoan_MismatchedCredit(t *testing.T) {
	app, evidence := prepareReconcileLoanTest(t)

	repo := &reconcileRepoStub{
		app:      app,
		revision: 10,
	}

	handler := &ReconcileCreditedLoanCommandHandler{
		Repo:     repo,
		Evidence: disburseEvidenceStub{e: evidence},
	}

	err := handler.Handle(
		context.Background(),
		ReconcileCreditedLoanCommand{
			LoanID:     app.Id,
			AccountID:  app.DisbursementAccountId,
			Amount:     app.RequestedAmount + 1,
			CreditedAt: time.Now().UTC(),
		},
	)

	if err == nil {
		t.Fatal("expected credit mismatch error")
	}

	if repo.appendCalls != 0 {
		t.Fatal("mismatched credit must not append")
	}
}

func TestReconcileCreditedLoan_UsesPreparedRate(t *testing.T) {
	app, _ := prepareReconcileLoanTest(t)

	// Lãi suất đã chốt trước khi cộng tiền.
	const preparedRate int64 = 1540

	preparedAt := app.ApprovedAt.Add(time.Second)

	if err := app.PrepareDisbursement(
		preparedAt,
		preparedRate,
	); err != nil {
		t.Fatal(err)
	}

	repo := &reconcileRepoStub{
		app:      app,
		revision: 11,
	}

	handler := &ReconcileCreditedLoanCommandHandler{
		Repo: repo,
		Evidence: disburseEvidenceStub{
			err: errors.New("evidence must not be read"),
		},
	}

	cmd := ReconcileCreditedLoanCommand{
		LoanID:     app.Id,
		AccountID:  app.DisbursementAccountId,
		Amount:     app.RequestedAmount,
		CreditedAt: preparedAt.Add(time.Second),
	}

	// Phục hồi phải thành công dù không đọc được Evidence.
	if err := handler.Handle(
		context.Background(),
		cmd,
	); err != nil {
		t.Fatalf("reconciliation failed: %v", err)
	}

	if repo.app.Status != domain.ApplicationStatusDisbursed {
		t.Fatal("loan was not disbursed")
	}

	// Phải sử dụng lãi suất đã chốt.
	if repo.app.AnnualRateBps != preparedRate {
		t.Fatalf(
			"expected rate %d, got %d",
			preparedRate,
			repo.app.AnnualRateBps,
		)
	}

	if repo.app.DisbursedAt == nil ||
		!repo.app.DisbursedAt.Equal(cmd.CreditedAt) {
		t.Fatal("incorrect disbursement timestamp")
	}

	if len(repo.app.RepaymentSchedule) != app.TermMonths {
		t.Fatal("incorrect repayment schedule")
	}

	// Chạy lại không được ghi LoanDisbursed lần hai.
	if err := handler.Handle(
		context.Background(),
		cmd,
	); err != nil {
		t.Fatal(err)
	}

	if repo.appendCalls != 1 {
		t.Fatalf(
			"expected one append, got %d",
			repo.appendCalls,
		)
	}
}
