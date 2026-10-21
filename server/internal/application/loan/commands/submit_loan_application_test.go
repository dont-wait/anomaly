package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
)

// Tạo hồ sơ draft có đủ 7 loại chứng từ bắt buộc.
func newReadyToSubmitLoan(t *testing.T) *loandomain.Application {
	t.Helper()

	app, err := loandomain.NewDraftApplication(
		"loan-submit-001",
		"LA-SUBMIT-001",
		"customer-001",
		"package-001",
		"account-001",
		100_000_000,
		24,
	)
	if err != nil {
		t.Fatalf("create draft failed: %v", err)
	}

	requiredTypes := []string{
		"IDENTITY_FRONT",
		"IDENTITY_BACK",
		"VNEID_PERSONAL",
		"VNEID_RESIDENCE",
		"VSSID_PARTICIPATION",
		"VSSID_DETAIL",
		"SALARY_STATEMENT_3_MONTHS",
	}

	for i, documentType := range requiredTypes {
		document := loandomain.ApplicationDocument{
			DocumentId: fmt.Sprintf("doc-%d", i+1),
			Type:       documentType,
			StorageKey: fmt.Sprintf("loans/doc-%d", i+1),
			MIMEType:   "application/pdf",
			SizeBytes:  1024,
			SHA256:     strings.Repeat("a", 64),
		}

		if err := app.AddDocument(document); err != nil {
			t.Fatalf(
				"add document %s failed: %v",
				documentType,
				err,
			)
		}
	}

	return app
}

func validSubmitLoanCommand() SubmitLoanApplicationCommand {
	return SubmitLoanApplicationCommand{
		ApplicationID: "loan-submit-001",
		CustomerID:    "customer-001",
	}
}

// 1. Nộp hồ sơ thành công.
func TestSubmitLoanApplication_Success(t *testing.T) {
	app := newReadyToSubmitLoan(t)

	repo := &addLoanDocumentRepoStub{
		app:      app,
		revision: 7,
	}

	handler := NewSubmitLoanApplicationCommandHandler(repo)

	result, err := handler.Handle(
		context.Background(),
		validSubmitLoanCommand(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected application")
	}

	if result.Status != loandomain.ApplicationStatusSubmitted {
		t.Fatalf("expected submitted, got %s", result.Status)
	}

	if result.SubmittedAt == nil || result.SubmittedAt.IsZero() {
		t.Fatal("submission timestamp is required")
	}

	if len(result.Documents) != 7 {
		t.Fatalf(
			"expected 7 documents, got %d",
			len(result.Documents),
		)
	}

	// Không thay đổi đối tượng gốc trước khi ghi event.
	if app.Status != loandomain.ApplicationStatusDraft {
		t.Fatal("original application was modified")
	}

	if app.SubmittedAt != nil {
		t.Fatal("original submittedAt must remain nil")
	}

	if repo.readCalls != 1 || repo.appendCalls != 1 {
		t.Fatal("expected one read and one append")
	}

	if repo.savedID != app.Id || repo.savedRev != 7 {
		t.Fatal("incorrect stream ID or revision")
	}

	if repo.savedType != loanApplicationSubmittedEvent {
		t.Fatalf("unexpected event type: %s", repo.savedType)
	}

	// Kiểm tra payload ghi vào EventStoreDB.
	data, err := json.Marshal(repo.savedPayload)
	if err != nil {
		t.Fatal(err)
	}

	var payload struct {
		SubmittedAt time.Time `json:"submittedAt"`
		UpdatedAt   time.Time `json:"updatedAt"`
	}

	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}

	if !payload.SubmittedAt.Equal(*result.SubmittedAt) {
		t.Fatal("incorrect submittedAt in event payload")
	}

	if !payload.UpdatedAt.Equal(result.UpdatedAt) {
		t.Fatal("incorrect updatedAt in event payload")
	}
}

// 2. Các yêu cầu Submit không hợp lệ.
func TestSubmitLoanApplication_InvalidRequests(t *testing.T) {
	tests := []struct {
		name   string
		modify func(
			*SubmitLoanApplicationCommand,
			*loandomain.Application,
			*addLoanDocumentRepoStub,
		)
	}{
		{
			name: "missing application ID",
			modify: func(
				cmd *SubmitLoanApplicationCommand,
				app *loandomain.Application,
				repo *addLoanDocumentRepoStub,
			) {
				cmd.ApplicationID = ""
			},
		},
		{
			name: "missing customer ID",
			modify: func(
				cmd *SubmitLoanApplicationCommand,
				app *loandomain.Application,
				repo *addLoanDocumentRepoStub,
			) {
				cmd.CustomerID = ""
			},
		},
		{
			name: "application not found",
			modify: func(
				cmd *SubmitLoanApplicationCommand,
				app *loandomain.Application,
				repo *addLoanDocumentRepoStub,
			) {
				repo.app = nil
			},
		},
		{
			name: "wrong customer",
			modify: func(
				cmd *SubmitLoanApplicationCommand,
				app *loandomain.Application,
				repo *addLoanDocumentRepoStub,
			) {
				cmd.CustomerID = "another-customer"
			},
		},
		{
			name: "missing required document",
			modify: func(
				cmd *SubmitLoanApplicationCommand,
				app *loandomain.Application,
				repo *addLoanDocumentRepoStub,
			) {
				app.Documents = app.Documents[:6]
			},
		},
		{
			name: "rejected required document",
			modify: func(
				cmd *SubmitLoanApplicationCommand,
				app *loandomain.Application,
				repo *addLoanDocumentRepoStub,
			) {
				app.Documents[6].VerificationStatus = "rejected"
			},
		},
		{
			name: "already submitted",
			modify: func(
				cmd *SubmitLoanApplicationCommand,
				app *loandomain.Application,
				repo *addLoanDocumentRepoStub,
			) {
				app.Status = loandomain.ApplicationStatusSubmitted
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newReadyToSubmitLoan(t)

			repo := &addLoanDocumentRepoStub{
				app:      app,
				revision: 7,
			}

			cmd := validSubmitLoanCommand()
			tt.modify(&cmd, app, repo)

			handler := NewSubmitLoanApplicationCommandHandler(repo)

			result, err := handler.Handle(
				context.Background(),
				cmd,
			)

			if err == nil {
				t.Fatal("expected validation error")
			}

			if result != nil {
				t.Fatal("invalid submit must not return application")
			}

			if repo.appendCalls != 0 {
				t.Fatal("invalid submit must not append event")
			}
		})
	}
}

// 3. EventStoreDB báo lỗi khi ghi sự kiện.
func TestSubmitLoanApplication_AppendFailure(t *testing.T) {
	appendErr := errors.New("revision conflict")

	repo := &addLoanDocumentRepoStub{
		app:       newReadyToSubmitLoan(t),
		revision:  7,
		appendErr: appendErr,
	}

	handler := NewSubmitLoanApplicationCommandHandler(repo)

	result, err := handler.Handle(
		context.Background(),
		validSubmitLoanCommand(),
	)

	if !errors.Is(err, appendErr) {
		t.Fatalf("expected append error, got %v", err)
	}

	if result != nil {
		t.Fatal("failed append must not return application")
	}

	if repo.appendCalls != 1 {
		t.Fatal("expected one append attempt")
	}

	if repo.app.Status != loandomain.ApplicationStatusDraft {
		t.Fatal("failed append modified original application")
	}
}

// 4. EventStoreDB báo lỗi khi đọc hồ sơ.
func TestSubmitLoanApplication_ReadFailure(t *testing.T) {
	readErr := errors.New("eventstore unavailable")

	repo := &addLoanDocumentRepoStub{
		readErr: readErr,
	}

	handler := NewSubmitLoanApplicationCommandHandler(repo)

	result, err := handler.Handle(
		context.Background(),
		validSubmitLoanCommand(),
	)

	if !errors.Is(err, readErr) {
		t.Fatalf("expected read error, got %v", err)
	}

	if result != nil {
		t.Fatal("expected nil application")
	}

	if repo.appendCalls != 0 {
		t.Fatal("read failure must not append event")
	}
}

// 5. Repository không được cấu hình.
func TestSubmitLoanApplication_MissingRepository(t *testing.T) {
	tests := []struct {
		name    string
		handler *SubmitLoanApplicationCommandHandler
	}{
		{
			name:    "nil handler",
			handler: nil,
		},
		{
			name:    "nil repository",
			handler: NewSubmitLoanApplicationCommandHandler(nil),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.handler.Handle(
				context.Background(),
				validSubmitLoanCommand(),
			)

			if err == nil {
				t.Fatal("expected dependency error")
			}

			if result != nil {
				t.Fatal("expected nil application")
			}
		})
	}
}
