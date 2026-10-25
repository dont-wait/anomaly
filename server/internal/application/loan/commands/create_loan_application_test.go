package commands

import (
	"context"
	"errors"
	"testing"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
)

// Mock Repository để test Command Handler
// mà không cần khởi động EventStoreDB.
type createLoanRepoStub struct {
	saved *loandomain.Application
	err   error
	calls int
}

func (r *createLoanRepoStub) Create(
	ctx context.Context,
	app *loandomain.Application,
) error {
	r.calls++

	if r.err != nil {
		return r.err
	}

	r.saved = app
	return nil
}

func (r *createLoanRepoStub) FindByID(
	ctx context.Context,
	id string,
) (*loandomain.Application, error) {
	if r.saved != nil && r.saved.Id == id {
		return r.saved, nil
	}

	return nil, nil
}

// Mock kiểm tra tài khoản nhận giải ngân.
type createLoanValidatorStub struct {
	calls      int
	customerID string
	accountID  string
	err        error
}

func (v *createLoanValidatorStub) ValidateDisbursementAccount(
	ctx context.Context,
	customerID string,
	accountID string,
) error {
	v.calls++
	v.customerID = customerID
	v.accountID = accountID

	return v.err
}

// Dữ liệu Command hợp lệ dùng chung cho các test.
func validCreateLoanCommand() CreateLoanApplicationCommand {
	return CreateLoanApplicationCommand{
		CustomerID:            "customer-001",
		LoanPackageID:         "package-001",
		DisbursementAccountID: "account-001",
		RequestedAmount:       100_000_000,
		TermMonths:            24,
	}
}

// 1. Tạo hồ sơ hợp lệ.
func TestCreateLoanApplication_Success(t *testing.T) {
	repo := &createLoanRepoStub{}
	validator := &createLoanValidatorStub{}

	handler := newCreateTestHandler(
		repo,
		validator,
	)

	app, err := handler.Handle(
		context.Background(),
		validCreateLoanCommand(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if app == nil {
		t.Fatal("expected application, got nil")
	}

	if app.Id == "" || app.ApplicationNo == "" {
		t.Fatal("application identifiers must be generated")
	}

	if app.Status != loandomain.ApplicationStatusDraft {
		t.Fatalf("expected draft, got %s", app.Status)
	}

	if app.CustomerId != "customer-001" {
		t.Fatal("incorrect customer ID")
	}

	if app.RequestedAmount != 100_000_000 {
		t.Fatal("incorrect requested amount")
	}

	if app.TermMonths != 24 {
		t.Fatal("incorrect loan term")
	}

	if len(app.Documents) != 0 {
		t.Fatal("new application must have no documents")
	}

	if app.CreatedAt.IsZero() || app.UpdatedAt.IsZero() {
		t.Fatal("application timestamps are required")
	}

	if validator.calls != 1 {
		t.Fatalf(
			"expected validator called once, got %d",
			validator.calls,
		)
	}

	if validator.customerID != "customer-001" ||
		validator.accountID != "account-001" {
		t.Fatal("incorrect validator arguments")
	}

	if repo.calls != 1 || repo.saved != app {
		t.Fatal("repository must save the created application")
	}
}

// 2. Từ chối dữ liệu đầu vào không hợp lệ.
func TestCreateLoanApplication_InvalidInput(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*CreateLoanApplicationCommand)
	}{
		{
			name: "unsupported term",
			modify: func(cmd *CreateLoanApplicationCommand) {
				cmd.TermMonths = 18
			},
		},
		{
			name: "zero term",
			modify: func(cmd *CreateLoanApplicationCommand) {
				cmd.TermMonths = 0
			},
		},
		{
			name: "zero amount",
			modify: func(cmd *CreateLoanApplicationCommand) {
				cmd.RequestedAmount = 0
			},
		},
		{
			name: "negative amount",
			modify: func(cmd *CreateLoanApplicationCommand) {
				cmd.RequestedAmount = -1
			},
		},
		{
			name: "missing customer",
			modify: func(cmd *CreateLoanApplicationCommand) {
				cmd.CustomerID = ""
			},
		},
		{
			name: "missing loan package",
			modify: func(cmd *CreateLoanApplicationCommand) {
				cmd.LoanPackageID = ""
			},
		},
		{
			name: "missing disbursement account",
			modify: func(cmd *CreateLoanApplicationCommand) {
				cmd.DisbursementAccountID = ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &createLoanRepoStub{}
			validator := &createLoanValidatorStub{}

			handler := newCreateTestHandler(
				repo,
				validator,
			)

			cmd := validCreateLoanCommand()
			tt.modify(&cmd)

			app, err := handler.Handle(
				context.Background(),
				cmd,
			)

			if err == nil {
				t.Fatal("expected validation error")
			}

			if app != nil {
				t.Fatal("invalid input must not create application")
			}

			if repo.calls != 0 {
				t.Fatal("repository must not be called")
			}

			if validator.calls != 0 {
				t.Fatal("validator must not be called")
			}
		})
	}
}

// 3. Tài khoản giải ngân không hợp lệ.
func TestCreateLoanApplication_InvalidDisbursementAccount(
	t *testing.T,
) {
	repo := &createLoanRepoStub{}

	accountErr := errors.New("account does not belong to customer")

	validator := &createLoanValidatorStub{
		err: accountErr,
	}

	handler := newCreateTestHandler(
		repo,
		validator,
	)

	app, err := handler.Handle(
		context.Background(),
		validCreateLoanCommand(),
	)

	if !errors.Is(err, accountErr) {
		t.Fatalf("expected account error, got %v", err)
	}

	if app != nil {
		t.Fatal("expected nil application")
	}

	if validator.calls != 1 {
		t.Fatal("validator must be called")
	}

	if repo.calls != 0 {
		t.Fatal("invalid account must not be saved")
	}
}

// 4. Repository báo lỗi khi tạo hồ sơ.
func TestCreateLoanApplication_RepositoryFailure(t *testing.T) {
	storageErr := errors.New("eventstore unavailable")

	repo := &createLoanRepoStub{
		err: storageErr,
	}

	validator := &createLoanValidatorStub{}

	handler := newCreateTestHandler(
		repo,
		validator,
	)

	app, err := handler.Handle(
		context.Background(),
		validCreateLoanCommand(),
	)

	if !errors.Is(err, storageErr) {
		t.Fatalf("expected storage error, got %v", err)
	}

	if app != nil {
		t.Fatal("failed save must not return successful application")
	}

	if repo.calls != 1 || validator.calls != 1 {
		t.Fatal("expected validation followed by repository call")
	}
}

// 5. Thiếu dependency.
func TestCreateLoanApplication_MissingDependency(t *testing.T) {
	tests := []struct {
		name    string
		handler *CreateLoanApplicationCommandHandler
	}{
		{
			name:    "nil handler",
			handler: nil,
		},
		{
			name: "nil repository",
			handler: newCreateTestHandler(
				nil,
				&createLoanValidatorStub{},
			),
		},
		{
			name: "nil validator",
			handler: newCreateTestHandler(
				&createLoanRepoStub{},
				nil,
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, err := tt.handler.Handle(
				context.Background(),
				validCreateLoanCommand(),
			)

			if err == nil {
				t.Fatal("expected dependency error")
			}

			if app != nil {
				t.Fatal("expected nil application")
			}
		})
	}
}

// Test fixtures explicitly allow the product used by legacy command tests.
// Production must install the Mongo-backed policy validator instead.
type createLoanPackagePolicyStub struct {
	err   error
	calls int
}

func (p *createLoanPackagePolicyStub) ValidateLoanPackage(_ context.Context, _ string, _ int64, _ int) error {
	p.calls++
	return p.err
}

func newCreateTestHandler(repo LoanApplicationRepository, validator LoanApplicantValidator) *CreateLoanApplicationCommandHandler {
	h := NewCreateLoanApplicationCommandHandler(repo, validator)
	h.PackagePolicy = &createLoanPackagePolicyStub{}
	return h
}

func TestCreateLoanApplicationRejectsInactiveOrOutOfRangePackageBeforeWrite(t *testing.T) {
	repo := &createLoanRepoStub{}
	h := newCreateTestHandler(repo, &createLoanValidatorStub{})
	h.PackagePolicy = &createLoanPackagePolicyStub{err: errors.New("inactive or outside package limits")}
	if _, err := h.Handle(context.Background(), validCreateLoanCommand()); err == nil {
		t.Fatal("policy must reject")
	}
	if repo.calls != 0 {
		t.Fatal("must not write event for ineligible package")
	}
}

func TestCreateLoanApplicationRequiresPackagePolicy(t *testing.T) {
	repo := &createLoanRepoStub{}
	h := newCreateTestHandler(repo, &createLoanValidatorStub{})
	h.PackagePolicy = nil
	if _, err := h.Handle(context.Background(), validCreateLoanCommand()); err == nil {
		t.Fatal("fail closed on missing policy")
	}
	if repo.calls != 0 {
		t.Fatal("must not create without catalog")
	}
}

func TestCreateLoanApplicationIdempotentRetrySurvivesPackageDeactivation(t *testing.T) {
	repo := &createLoanRepoStub{}
	h := newCreateTestHandler(repo, &createLoanValidatorStub{})
	cmd := validCreateLoanCommand()
	cmd.IdempotencyKey = "123e4567-e89b-12d3-a456-426614174000"
	first, err := h.Handle(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	policy := &createLoanPackagePolicyStub{err: errors.New("deactivated")}
	h.PackagePolicy = policy
	second, err := h.Handle(context.Background(), cmd)
	if err != nil || second.Id != first.Id || repo.calls != 1 || policy.calls != 0 {
		t.Fatalf("idempotent retry should reuse saved application: err=%v", err)
	}
}
