package commands

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
)

// Giả lập bộ kiểm tra quyền thẩm định.
type startLoanReviewAuthorizerStub struct {
	calls   int
	actorID string
	err     error
}

func (a *startLoanReviewAuthorizerStub) AuthorizeLoanReview(
	ctx context.Context,
	actorID string,
) error {
	a.calls++
	a.actorID = actorID
	return a.err
}

// Chuẩn bị hồ sơ đã nộp, sẵn sàng thẩm định.
func newSubmittedLoanForReviewTest(
	t *testing.T,
) *loandomain.Application {
	t.Helper()

	// Tái sử dụng helper từ Bước 41.
	app := newReadyToSubmitLoan(t)

	if err := app.Submit(); err != nil {
		t.Fatalf("submit application: %v", err)
	}

	return app
}

func validStartLoanReviewCommand() StartLoanReviewCommand {
	return StartLoanReviewCommand{
		ApplicationID: "loan-submit-001",
		ActorID:       "STAFF001",
	}
}

// 1. Nhân viên có quyền bắt đầu thẩm định.
func TestStartLoanReview_Success(t *testing.T) {
	app := newSubmittedLoanForReviewTest(t)

	repo := &addLoanDocumentRepoStub{
		app:      app,
		revision: 8,
	}
	authorizer := &startLoanReviewAuthorizerStub{}

	handler := NewStartLoanReviewCommandHandler(
		repo,
		authorizer,
	)

	result, err := handler.Handle(
		context.Background(),
		validStartLoanReviewCommand(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected application")
	}

	if result.Status != loandomain.ApplicationStatusUnderReview {
		t.Fatalf("expected under_review, got %s", result.Status)
	}

	if result.ReviewStartedAt == nil ||
		result.ReviewStartedAt.IsZero() {
		t.Fatal("review timestamp is required")
	}

	// Aggregate đọc từ repository không được sửa trực tiếp.
	if app.Status != loandomain.ApplicationStatusSubmitted {
		t.Fatal("original application was modified")
	}

	if app.ReviewStartedAt != nil {
		t.Fatal("original review timestamp must remain nil")
	}

	// Kiểm tra quyền được gọi.
	if authorizer.calls != 1 ||
		authorizer.actorID != "STAFF001" {
		t.Fatal("authorizer received incorrect actor")
	}

	if repo.readCalls != 1 || repo.appendCalls != 1 {
		t.Fatal("expected one read and one append")
	}

	if repo.savedID != app.Id || repo.savedRev != 8 {
		t.Fatal("incorrect stream ID or revision")
	}

	if repo.savedType != loanReviewStartedEvent {
		t.Fatalf("unexpected event type: %s", repo.savedType)
	}

	// Kiểm tra payload sự kiện.
	data, err := json.Marshal(repo.savedPayload)
	if err != nil {
		t.Fatal(err)
	}

	var payload struct {
		ReviewStartedAt time.Time `json:"reviewStartedAt"`
		UpdatedAt       time.Time `json:"updatedAt"`
	}

	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}

	if !payload.ReviewStartedAt.Equal(*result.ReviewStartedAt) {
		t.Fatal("incorrect reviewStartedAt in event payload")
	}

	if !payload.UpdatedAt.Equal(result.UpdatedAt) {
		t.Fatal("incorrect updatedAt in event payload")
	}
}

// 2. Kiểm tra các yêu cầu không hợp lệ.
func TestStartLoanReview_InvalidRequests(t *testing.T) {
	tests := []struct {
		name   string
		modify func(
			*StartLoanReviewCommand,
			*loandomain.Application,
			*addLoanDocumentRepoStub,
		)
	}{
		{
			name: "missing application ID",
			modify: func(
				cmd *StartLoanReviewCommand,
				app *loandomain.Application,
				repo *addLoanDocumentRepoStub,
			) {
				cmd.ApplicationID = ""
			},
		},
		{
			name: "missing actor ID",
			modify: func(
				cmd *StartLoanReviewCommand,
				app *loandomain.Application,
				repo *addLoanDocumentRepoStub,
			) {
				cmd.ActorID = ""
			},
		},
		{
			name: "application not found",
			modify: func(
				cmd *StartLoanReviewCommand,
				app *loandomain.Application,
				repo *addLoanDocumentRepoStub,
			) {
				repo.app = nil
			},
		},
		{
			name: "application ID mismatch",
			modify: func(
				cmd *StartLoanReviewCommand,
				app *loandomain.Application,
				repo *addLoanDocumentRepoStub,
			) {
				cmd.ApplicationID = "another-loan"
			},
		},
		{
			name: "application still draft",
			modify: func(
				cmd *StartLoanReviewCommand,
				app *loandomain.Application,
				repo *addLoanDocumentRepoStub,
			) {
				app.Status = loandomain.ApplicationStatusDraft
			},
		},
		{
			name: "already under review",
			modify: func(
				cmd *StartLoanReviewCommand,
				app *loandomain.Application,
				repo *addLoanDocumentRepoStub,
			) {
				app.Status = loandomain.ApplicationStatusUnderReview
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newSubmittedLoanForReviewTest(t)

			repo := &addLoanDocumentRepoStub{
				app:      app,
				revision: 8,
			}
			authorizer := &startLoanReviewAuthorizerStub{}

			cmd := validStartLoanReviewCommand()
			tt.modify(&cmd, app, repo)

			handler := NewStartLoanReviewCommandHandler(
				repo,
				authorizer,
			)

			result, err := handler.Handle(
				context.Background(),
				cmd,
			)

			if err == nil {
				t.Fatal("expected validation error")
			}

			if result != nil {
				t.Fatal("invalid request must not return application")
			}

			if repo.appendCalls != 0 {
				t.Fatal("invalid request must not append event")
			}
		})
	}
}

// 3. Nhân viên không có quyền thẩm định.
func TestStartLoanReview_Unauthorized(t *testing.T) {
	authErr := errors.New("employee is not authorized")

	repo := &addLoanDocumentRepoStub{
		app: newSubmittedLoanForReviewTest(t),
	}

	authorizer := &startLoanReviewAuthorizerStub{
		err: authErr,
	}

	handler := NewStartLoanReviewCommandHandler(
		repo,
		authorizer,
	)

	result, err := handler.Handle(
		context.Background(),
		validStartLoanReviewCommand(),
	)

	if !errors.Is(err, authErr) {
		t.Fatalf("expected authorization error, got %v", err)
	}

	if result != nil {
		t.Fatal("unauthorized request returned application")
	}

	if authorizer.calls != 1 {
		t.Fatal("authorizer must be called")
	}

	if repo.readCalls != 0 || repo.appendCalls != 0 {
		t.Fatal("unauthorized request must not access repository")
	}
}

// 4. EventStoreDB không đọc được hồ sơ.
func TestStartLoanReview_ReadFailure(t *testing.T) {
	readErr := errors.New("eventstore unavailable")

	repo := &addLoanDocumentRepoStub{
		readErr: readErr,
	}
	authorizer := &startLoanReviewAuthorizerStub{}

	handler := NewStartLoanReviewCommandHandler(
		repo,
		authorizer,
	)

	result, err := handler.Handle(
		context.Background(),
		validStartLoanReviewCommand(),
	)

	if !errors.Is(err, readErr) {
		t.Fatalf("expected read error, got %v", err)
	}

	if result != nil || repo.appendCalls != 0 {
		t.Fatal("read failure must not append event")
	}
}

// 5. EventStoreDB báo lỗi khi ghi sự kiện.
func TestStartLoanReview_AppendFailure(t *testing.T) {
	appendErr := errors.New("revision conflict")

	app := newSubmittedLoanForReviewTest(t)

	repo := &addLoanDocumentRepoStub{
		app:       app,
		revision:  8,
		appendErr: appendErr,
	}

	authorizer := &startLoanReviewAuthorizerStub{}

	handler := NewStartLoanReviewCommandHandler(
		repo,
		authorizer,
	)

	result, err := handler.Handle(
		context.Background(),
		validStartLoanReviewCommand(),
	)

	if !errors.Is(err, appendErr) {
		t.Fatalf("expected append error, got %v", err)
	}

	if result != nil {
		t.Fatal("failed append must not return successful application")
	}

	if repo.appendCalls != 1 {
		t.Fatal("expected one append attempt")
	}

	if app.Status != loandomain.ApplicationStatusSubmitted {
		t.Fatal("failed append modified original application")
	}
}

// 6. Kiểm tra dependency.
func TestStartLoanReview_MissingDependency(t *testing.T) {
	tests := []struct {
		name    string
		handler *StartLoanReviewCommandHandler
	}{
		{
			name:    "nil handler",
			handler: nil,
		},
		{
			name: "nil repository",
			handler: NewStartLoanReviewCommandHandler(
				nil,
				&startLoanReviewAuthorizerStub{},
			),
		},
		{
			name: "nil authorizer",
			handler: NewStartLoanReviewCommandHandler(
				&addLoanDocumentRepoStub{},
				nil,
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.handler.Handle(
				context.Background(),
				validStartLoanReviewCommand(),
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
