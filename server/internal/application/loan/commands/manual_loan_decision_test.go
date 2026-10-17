package commands

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
)

// Authorizer giả lập kiểm tra quyền quyết định thủ công.
type manualLoanAuthorizerStub struct {
	calls   int
	actorID string
	err     error
}

func (a *manualLoanAuthorizerStub) AuthorizeLoanManualDecision(
	ctx context.Context,
	actorID string,
) error {
	a.calls++
	a.actorID = actorID
	return a.err
}

// Tạo hồ sơ đã được chấm điểm và đang chờ
// nhân viên đưa ra quyết định thủ công.
func newLoanAwaitingManualDecision(
	t *testing.T,
) *loandomain.Application {
	t.Helper()

	// Tái sử dụng helper từ Bước 45.
	app := newLoanForAssessmentTest(t)

	input := validAssessmentCommandInput()

	// Chưa xác minh đủ chứng từ nên cần manual_review.
	input.DocumentsVerified = false

	result, err := app.ApplyAssessment(input)
	if err != nil {
		t.Fatalf("prepare manual assessment: %v", err)
	}

	if result.Decision != loandomain.AssessmentManualReview {
		t.Fatalf(
			"expected manual_review, got %s",
			result.Decision,
		)
	}

	if app.Status != loandomain.ApplicationStatusUnderReview {
		t.Fatalf(
			"expected under_review, got %s",
			app.Status,
		)
	}

	return app
}

func validManualLoanCommand() ManualLoanDecisionCommand {
	return ManualLoanDecisionCommand{
		ApplicationID: "loan-submit-001",
		ActorID:       "STAFF001",
		Decision:      loandomain.ApplicationStatusApproved,
		Reason:        "Da kiem tra va xac minh ho so",
	}
}

// 1. Kiểm tra phê duyệt và từ chối thủ công.
func TestManualLoanDecision_Success(t *testing.T) {
	tests := []struct {
		name      string
		decision  loandomain.ApplicationStatus
		wantEvent string
	}{
		{
			name:      "manual approval",
			decision:  loandomain.ApplicationStatusApproved,
			wantEvent: loanApplicationApprovedEvent,
		},
		{
			name:      "manual rejection",
			decision:  loandomain.ApplicationStatusRejected,
			wantEvent: loanApplicationRejectedEvent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newLoanAwaitingManualDecision(t)

			repo := &addLoanDocumentRepoStub{
				app:      app,
				revision: 10,
			}

			authorizer := &manualLoanAuthorizerStub{}

			handler := NewManualLoanDecisionCommandHandler(
				repo,
				authorizer,
			)

			cmd := validManualLoanCommand()
			cmd.Decision = tt.decision

			result, err := handler.Handle(
				context.Background(),
				cmd,
			)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("expected application")
			}

			if result.Status != tt.decision {
				t.Fatalf(
					"expected status %s, got %s",
					tt.decision,
					result.Status,
				)
			}

			if result.ManualReviewerId != "STAFF001" {
				t.Fatal("incorrect reviewer ID")
			}

			if result.ManualDecisionReason != cmd.Reason {
				t.Fatal("incorrect decision reason")
			}

			if result.ManualDecidedAt == nil ||
				result.ManualDecidedAt.IsZero() {
				t.Fatal("manual decision timestamp is required")
			}

			// Aggregate ban đầu không được thay đổi.
			if app.Status != loandomain.ApplicationStatusUnderReview {
				t.Fatal("original application was modified")
			}

			if app.ManualDecidedAt != nil ||
				app.ManualReviewerId != "" {
				t.Fatal("original manual decision fields were modified")
			}

			// Kiểm tra Authorizer.
			if authorizer.calls != 1 ||
				authorizer.actorID != "STAFF001" {
				t.Fatal("incorrect authorization call")
			}

			// Kiểm tra Repository.
			if repo.readCalls != 1 ||
				repo.appendCalls != 1 {
				t.Fatal("expected one read and one append")
			}

			if repo.savedID != app.Id ||
				repo.savedRev != 10 {
				t.Fatal("incorrect stream ID or revision")
			}

			if repo.savedType != tt.wantEvent {
				t.Fatalf(
					"expected event %s, got %s",
					tt.wantEvent,
					repo.savedType,
				)
			}

			// Kiểm tra nội dung event.
			data, err := json.Marshal(repo.savedPayload)
			if err != nil {
				t.Fatal(err)
			}

			var payload struct {
				DecidedAt            time.Time  `json:"decidedAt"`
				UpdatedAt            time.Time  `json:"updatedAt"`
				ManualReviewerId     string     `json:"manualReviewerId"`
				ManualDecisionReason string     `json:"manualDecisionReason"`
				ManualDecidedAt      *time.Time `json:"manualDecidedAt"`
			}

			if err := json.Unmarshal(data, &payload); err != nil {
				t.Fatal(err)
			}

			if payload.ManualReviewerId != "STAFF001" {
				t.Fatal("event has incorrect reviewer ID")
			}

			if payload.ManualDecisionReason != cmd.Reason {
				t.Fatal("event has incorrect decision reason")
			}

			if payload.ManualDecidedAt == nil {
				t.Fatal("event missing manual decision time")
			}

			if !payload.ManualDecidedAt.Equal(
				*result.ManualDecidedAt,
			) {
				t.Fatal("incorrect manual decision timestamp")
			}

			if !payload.DecidedAt.Equal(
				*result.ManualDecidedAt,
			) {
				t.Fatal("decision timestamps do not match")
			}

			if !payload.UpdatedAt.Equal(result.UpdatedAt) {
				t.Fatal("incorrect updatedAt")
			}

			// Kiểm tra timestamp theo từng quyết định.
			switch tt.decision {
			case loandomain.ApplicationStatusApproved:
				if result.ApprovedAt == nil ||
					!result.ApprovedAt.Equal(payload.DecidedAt) {
					t.Fatal("incorrect approval timestamp")
				}

				if result.RejectedAt != nil {
					t.Fatal("approved loan must not be rejected")
				}

			case loandomain.ApplicationStatusRejected:
				if result.RejectedAt == nil ||
					!result.RejectedAt.Equal(payload.DecidedAt) {
					t.Fatal("incorrect rejection timestamp")
				}

				if result.ApprovedAt != nil {
					t.Fatal("rejected loan must not be approved")
				}
			}
		})
	}
}

// 2. Kiểm tra các yêu cầu không hợp lệ.
func TestManualLoanDecision_InvalidRequests(t *testing.T) {
	tests := []struct {
		name   string
		modify func(
			*ManualLoanDecisionCommand,
			*addLoanDocumentRepoStub,
		)
	}{
		{
			name: "missing application ID",
			modify: func(
				cmd *ManualLoanDecisionCommand,
				repo *addLoanDocumentRepoStub,
			) {
				cmd.ApplicationID = ""
			},
		},
		{
			name: "missing actor ID",
			modify: func(
				cmd *ManualLoanDecisionCommand,
				repo *addLoanDocumentRepoStub,
			) {
				cmd.ActorID = ""
			},
		},
		{
			name: "missing reason",
			modify: func(
				cmd *ManualLoanDecisionCommand,
				repo *addLoanDocumentRepoStub,
			) {
				cmd.Reason = "   "
			},
		},
		{
			name: "invalid decision",
			modify: func(
				cmd *ManualLoanDecisionCommand,
				repo *addLoanDocumentRepoStub,
			) {
				cmd.Decision = loandomain.ApplicationStatusUnderReview
			},
		},
		{
			name: "application not found",
			modify: func(
				cmd *ManualLoanDecisionCommand,
				repo *addLoanDocumentRepoStub,
			) {
				repo.app = nil
			},
		},
		{
			name: "application ID mismatch",
			modify: func(
				cmd *ManualLoanDecisionCommand,
				repo *addLoanDocumentRepoStub,
			) {
				cmd.ApplicationID = "another-loan"
			},
		},
		{
			name: "application still submitted",
			modify: func(
				cmd *ManualLoanDecisionCommand,
				repo *addLoanDocumentRepoStub,
			) {
				repo.app.Status = loandomain.ApplicationStatusSubmitted
			},
		},
		{
			name: "assessment missing",
			modify: func(
				cmd *ManualLoanDecisionCommand,
				repo *addLoanDocumentRepoStub,
			) {
				repo.app.AssessedAt = nil
			},
		},
		{
			name: "assessment is not manual review",
			modify: func(
				cmd *ManualLoanDecisionCommand,
				repo *addLoanDocumentRepoStub,
			) {
				repo.app.AssessmentDecision = loandomain.AssessmentAutoEligible
			},
		},
		{
			name: "already approved",
			modify: func(
				cmd *ManualLoanDecisionCommand,
				repo *addLoanDocumentRepoStub,
			) {
				repo.app.Status = loandomain.ApplicationStatusApproved
			},
		},
		{
			name: "already rejected",
			modify: func(
				cmd *ManualLoanDecisionCommand,
				repo *addLoanDocumentRepoStub,
			) {
				repo.app.Status = loandomain.ApplicationStatusRejected
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &addLoanDocumentRepoStub{
				app:      newLoanAwaitingManualDecision(t),
				revision: 10,
			}

			authorizer := &manualLoanAuthorizerStub{}

			cmd := validManualLoanCommand()
			tt.modify(&cmd, repo)

			handler := NewManualLoanDecisionCommandHandler(
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
				t.Fatal("invalid request returned application")
			}

			if repo.appendCalls != 0 {
				t.Fatal("invalid request must not append event")
			}
		})
	}
}

// 3. Nhân viên không có quyền đưa ra quyết định.
func TestManualLoanDecision_Unauthorized(t *testing.T) {
	authErr := errors.New("manual decision permission denied")

	repo := &addLoanDocumentRepoStub{
		app: newLoanAwaitingManualDecision(t),
	}

	authorizer := &manualLoanAuthorizerStub{
		err: authErr,
	}

	handler := NewManualLoanDecisionCommandHandler(
		repo,
		authorizer,
	)

	result, err := handler.Handle(
		context.Background(),
		validManualLoanCommand(),
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
		t.Fatal("unauthorized request accessed repository")
	}
}

// 4. EventStoreDB lỗi khi đọc hoặc ghi dữ liệu.
func TestManualLoanDecision_RepositoryFailures(t *testing.T) {
	tests := []struct {
		name      string
		readErr   error
		appendErr error
	}{
		{
			name:    "read failure",
			readErr: errors.New("eventstore read failed"),
		},
		{
			name:      "append failure",
			appendErr: errors.New("revision conflict"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newLoanAwaitingManualDecision(t)

			repo := &addLoanDocumentRepoStub{
				app:       app,
				revision:  10,
				readErr:   tt.readErr,
				appendErr: tt.appendErr,
			}

			authorizer := &manualLoanAuthorizerStub{}

			handler := NewManualLoanDecisionCommandHandler(
				repo,
				authorizer,
			)

			result, err := handler.Handle(
				context.Background(),
				validManualLoanCommand(),
			)

			expectedErr := tt.readErr
			if expectedErr == nil {
				expectedErr = tt.appendErr
			}

			if !errors.Is(err, expectedErr) {
				t.Fatalf(
					"expected error %v, got %v",
					expectedErr,
					err,
				)
			}

			if result != nil {
				t.Fatal("repository failure returned application")
			}

			if app.Status != loandomain.ApplicationStatusUnderReview {
				t.Fatal("repository failure changed original status")
			}

			if app.ManualDecidedAt != nil {
				t.Fatal("repository failure changed original decision")
			}

			if tt.readErr != nil && repo.appendCalls != 0 {
				t.Fatal("read failure must not append event")
			}

			if tt.appendErr != nil && repo.appendCalls != 1 {
				t.Fatal("expected one append attempt")
			}
		})
	}
}

// 5. Thiếu dependency.
func TestManualLoanDecision_MissingDependency(t *testing.T) {
	repo := &addLoanDocumentRepoStub{}
	authorizer := &manualLoanAuthorizerStub{}

	tests := []struct {
		name    string
		handler *ManualLoanDecisionCommandHandler
	}{
		{
			name:    "nil handler",
			handler: nil,
		},
		{
			name: "nil repository",
			handler: NewManualLoanDecisionCommandHandler(
				nil,
				authorizer,
			),
		},
		{
			name: "nil authorizer",
			handler: NewManualLoanDecisionCommandHandler(
				repo,
				nil,
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.handler.Handle(
				context.Background(),
				validManualLoanCommand(),
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
