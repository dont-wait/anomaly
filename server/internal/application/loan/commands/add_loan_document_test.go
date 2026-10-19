package commands

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
)

// Repository giả lập.
type addLoanDocumentRepoStub struct {
	app          *loandomain.Application
	revision     uint64
	readErr      error
	appendErr    error
	readCalls    int
	appendCalls  int
	savedID      string
	savedRev     uint64
	savedType    string
	savedPayload any
}

func (r *addLoanDocumentRepoStub) FindByIDWithRevision(
	ctx context.Context,
	id string,
) (*loandomain.Application, uint64, error) {
	r.readCalls++

	if r.readErr != nil {
		return nil, 0, r.readErr
	}

	return r.app, r.revision, nil
}

func (r *addLoanDocumentRepoStub) Append(
	ctx context.Context,
	id string,
	revision uint64,
	eventType string,
	payload any,
) error {
	r.appendCalls++
	r.savedID = id
	r.savedRev = revision
	r.savedType = eventType
	r.savedPayload = payload

	return r.appendErr
}

// RustFS verifier giả lập.
type addLoanDocumentVerifierStub struct {
	calls      int
	customerID string
	appID      string
	document   loandomain.ApplicationDocument
	err        error
}

func (v *addLoanDocumentVerifierStub) VerifyLoanDocument(
	ctx context.Context,
	customerID string,
	applicationID string,
	document loandomain.ApplicationDocument,
) error {
	v.calls++
	v.customerID = customerID
	v.appID = applicationID
	v.document = document

	return v.err
}

// Chuẩn bị hồ sơ draft hợp lệ.
func newAddLoanDocumentDraft(t *testing.T) *loandomain.Application {
	t.Helper()

	app, err := loandomain.NewDraftApplication(
		"loan-001",
		"LA001",
		"customer-001",
		"package-001",
		"account-001",
		100_000_000,
		24,
	)
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}

	return app
}

// Command hợp lệ.
func validAddLoanDocumentCommand() AddLoanDocumentCommand {
	return AddLoanDocumentCommand{
		ApplicationID: "loan-001",
		CustomerID:    "customer-001",
		Document: loandomain.ApplicationDocument{
			DocumentId: "document-001",
			Type:       "IDENTITY_FRONT",
			StorageKey: "loans/loan-001/identity-front",
			MIMEType:   "image/jpeg",
			SizeBytes:  2048,
			SHA256:     strings.Repeat("a", 64),
		},
	}
}

// 1. Thêm chứng từ thành công.
func TestAddLoanDocument_Success(t *testing.T) {
	app := newAddLoanDocumentDraft(t)

	repo := &addLoanDocumentRepoStub{
		app:      app,
		revision: 3,
	}
	verifier := &addLoanDocumentVerifierStub{}

	handler := NewAddLoanDocumentCommandHandler(
		repo,
		verifier,
	)

	result, err := handler.Handle(
		context.Background(),
		validAddLoanDocumentCommand(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil || len(result.Documents) != 1 {
		t.Fatal("expected application with one document")
	}

	doc := result.Documents[0]

	if doc.DocumentId != "document-001" {
		t.Fatal("incorrect document ID")
	}

	if doc.VerificationStatus != "pending" {
		t.Fatal("new document must be pending")
	}

	if doc.UploadedAt.IsZero() {
		t.Fatal("document upload timestamp is required")
	}

	if result.Status != loandomain.ApplicationStatusDraft {
		t.Fatal("application must remain draft")
	}

	// Đảm bảo không sửa trực tiếp aggregate vừa đọc.
	if len(app.Documents) != 0 {
		t.Fatal("original application was modified")
	}

	if verifier.calls != 1 ||
		verifier.customerID != app.CustomerId ||
		verifier.appID != app.Id {
		t.Fatal("storage verifier received incorrect arguments")
	}

	if verifier.document.DocumentId != doc.DocumentId {
		t.Fatal("storage verifier received wrong document")
	}

	if repo.appendCalls != 1 {
		t.Fatal("expected one event append")
	}

	if repo.savedID != app.Id || repo.savedRev != 3 {
		t.Fatal("incorrect stream ID or expected revision")
	}

	if repo.savedType != "LoanDocumentAdded" {
		t.Fatal("incorrect event type")
	}

	// Kiểm tra payload thực sự được gửi tới Repository.
	data, err := json.Marshal(repo.savedPayload)
	if err != nil {
		t.Fatal(err)
	}

	var payload struct {
		Document  loandomain.ApplicationDocument `json:"document"`
		UpdatedAt time.Time                      `json:"updatedAt"`
	}

	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}

	if payload.Document.DocumentId != doc.DocumentId ||
		!payload.UpdatedAt.Equal(result.UpdatedAt) {
		t.Fatal("event payload does not match domain state")
	}
}

// 2. Kiểm tra các yêu cầu không hợp lệ.
func TestAddLoanDocument_InvalidRequests(t *testing.T) {
	tests := []struct {
		name  string
		setup func(
			*loandomain.Application,
			*AddLoanDocumentCommand,
			*addLoanDocumentRepoStub,
		)
	}{
		{
			name: "missing application ID",
			setup: func(
				app *loandomain.Application,
				cmd *AddLoanDocumentCommand,
				repo *addLoanDocumentRepoStub,
			) {
				cmd.ApplicationID = ""
			},
		},
		{
			name: "missing customer ID",
			setup: func(
				app *loandomain.Application,
				cmd *AddLoanDocumentCommand,
				repo *addLoanDocumentRepoStub,
			) {
				cmd.CustomerID = ""
			},
		},
		{
			name: "application not found",
			setup: func(
				app *loandomain.Application,
				cmd *AddLoanDocumentCommand,
				repo *addLoanDocumentRepoStub,
			) {
				repo.app = nil
			},
		},
		{
			name: "wrong customer",
			setup: func(
				app *loandomain.Application,
				cmd *AddLoanDocumentCommand,
				repo *addLoanDocumentRepoStub,
			) {
				cmd.CustomerID = "another-customer"
			},
		},
		{
			name: "application already submitted",
			setup: func(
				app *loandomain.Application,
				cmd *AddLoanDocumentCommand,
				repo *addLoanDocumentRepoStub,
			) {
				app.Status = loandomain.ApplicationStatusSubmitted
			},
		},
		{
			name: "duplicate document",
			setup: func(
				app *loandomain.Application,
				cmd *AddLoanDocumentCommand,
				repo *addLoanDocumentRepoStub,
			) {
				app.Documents = append(app.Documents, cmd.Document)
			},
		},
		{
			name: "invalid document metadata",
			setup: func(
				app *loandomain.Application,
				cmd *AddLoanDocumentCommand,
				repo *addLoanDocumentRepoStub,
			) {
				cmd.Document.StorageKey = ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newAddLoanDocumentDraft(t)
			repo := &addLoanDocumentRepoStub{
				app:      app,
				revision: 0,
			}
			verifier := &addLoanDocumentVerifierStub{}

			cmd := validAddLoanDocumentCommand()
			tt.setup(app, &cmd, repo)

			handler := NewAddLoanDocumentCommandHandler(
				repo,
				verifier,
			)

			result, err := handler.Handle(
				context.Background(),
				cmd,
			)

			if err == nil {
				t.Fatal("expected validation error")
			}

			if result != nil {
				t.Fatal("expected nil application")
			}

			if repo.appendCalls != 0 {
				t.Fatal("invalid request must not append event")
			}

			if verifier.calls != 0 {
				t.Fatal("invalid request must not verify storage")
			}
		})
	}
}

// 3. RustFS xác minh chứng từ thất bại.
func TestAddLoanDocument_StorageVerificationFailure(t *testing.T) {
	storageErr := errors.New("document not found in storage")

	repo := &addLoanDocumentRepoStub{
		app: newAddLoanDocumentDraft(t),
	}

	verifier := &addLoanDocumentVerifierStub{
		err: storageErr,
	}

	handler := NewAddLoanDocumentCommandHandler(
		repo,
		verifier,
	)

	result, err := handler.Handle(
		context.Background(),
		validAddLoanDocumentCommand(),
	)

	if !errors.Is(err, storageErr) {
		t.Fatalf("expected storage error, got %v", err)
	}

	if result != nil || repo.appendCalls != 0 {
		t.Fatal("failed verification must not append event")
	}

	if verifier.calls != 1 {
		t.Fatal("expected storage verifier to be called")
	}
}

// 4. EventStoreDB trả lỗi khi ghi sự kiện.
func TestAddLoanDocument_AppendFailure(t *testing.T) {
	appendErr := errors.New("revision conflict")

	repo := &addLoanDocumentRepoStub{
		app:       newAddLoanDocumentDraft(t),
		appendErr: appendErr,
	}

	verifier := &addLoanDocumentVerifierStub{}

	handler := NewAddLoanDocumentCommandHandler(
		repo,
		verifier,
	)

	result, err := handler.Handle(
		context.Background(),
		validAddLoanDocumentCommand(),
	)

	if !errors.Is(err, appendErr) {
		t.Fatalf("expected append error, got %v", err)
	}

	if result != nil {
		t.Fatal("failed append must not return successful application")
	}

	if repo.appendCalls != 1 || verifier.calls != 1 {
		t.Fatal("expected verification followed by append")
	}
}

// 5. Repository đọc hồ sơ thất bại.
func TestAddLoanDocument_ReadFailure(t *testing.T) {
	readErr := errors.New("eventstore unavailable")

	repo := &addLoanDocumentRepoStub{
		readErr: readErr,
	}
	verifier := &addLoanDocumentVerifierStub{}

	handler := NewAddLoanDocumentCommandHandler(
		repo,
		verifier,
	)

	result, err := handler.Handle(
		context.Background(),
		validAddLoanDocumentCommand(),
	)

	if !errors.Is(err, readErr) {
		t.Fatalf("expected read error, got %v", err)
	}

	if result != nil || repo.appendCalls != 0 {
		t.Fatal("read failure must not append event")
	}

	if verifier.calls != 0 {
		t.Fatal("storage verifier must not run after read failure")
	}
}

// 6. Kiểm tra dependency.
func TestAddLoanDocument_MissingDependency(t *testing.T) {
	tests := []struct {
		name    string
		handler *AddLoanDocumentCommandHandler
	}{
		{
			name:    "nil handler",
			handler: nil,
		},
		{
			name: "nil repository",
			handler: NewAddLoanDocumentCommandHandler(
				nil,
				&addLoanDocumentVerifierStub{},
			),
		},
		{
			name: "nil verifier",
			handler: NewAddLoanDocumentCommandHandler(
				&addLoanDocumentRepoStub{},
				nil,
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.handler.Handle(
				context.Background(),
				validAddLoanDocumentCommand(),
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
