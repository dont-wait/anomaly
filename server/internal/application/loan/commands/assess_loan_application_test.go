package commands

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
)

// Repository giả lập.
type assessLoanRepoStub struct {
	app         *loandomain.Application
	revision    uint64
	readErr     error
	appendErr   error
	readCalls   int
	appendCalls int
	savedID     string
	savedRev    uint64
	savedEvents []LoanAppendEvent
}

func (r *assessLoanRepoStub) FindByIDWithRevision(
	ctx context.Context,
	id string,
) (*loandomain.Application, uint64, error) {
	r.readCalls++

	if r.readErr != nil {
		return nil, 0, r.readErr
	}

	return r.app, r.revision, nil
}

func (r *assessLoanRepoStub) AppendBatch(
	ctx context.Context,
	id string,
	revision uint64,
	events ...LoanAppendEvent,
) error {
	r.appendCalls++
	r.savedID = id
	r.savedRev = revision
	r.savedEvents = append(
		[]LoanAppendEvent(nil),
		events...,
	)

	return r.appendErr
}

// Authorizer giả lập.
type assessLoanAuthorizerStub struct {
	calls   int
	actorID string
	err     error
}

func (a *assessLoanAuthorizerStub) AuthorizeLoanAssessment(
	ctx context.Context,
	actorID string,
) error {
	a.calls++
	a.actorID = actorID
	return a.err
}

// DataProvider giả lập.
type assessLoanDataProviderStub struct {
	calls int
	app   *loandomain.Application
	input loandomain.LoanAssessmentInput
	err   error
}

func (p *assessLoanDataProviderStub) BuildAssessmentInput(
	ctx context.Context,
	app *loandomain.Application,
) (loandomain.LoanAssessmentInput, error) {
	p.calls++
	p.app = app

	return p.input, p.err
}

// Tạo hồ sơ đang under_review.
func newLoanForAssessmentTest(
	t *testing.T,
) *loandomain.Application {
	t.Helper()

	// Dùng lại helper từ Bước 43.
	app := newSubmittedLoanForReviewTest(t)

	if err := app.StartReview(); err != nil {
		t.Fatalf("start review failed: %v", err)
	}

	return app
}

// Dữ liệu thẩm định hợp lệ.
//
// Thu nhập: 10 triệu/tháng.
// Tổng nghĩa vụ trả nợ: 3,5 triệu/tháng.
// DTI = 35% -> 30 điểm.
// BHXH 24 tháng -> 25 điểm.
// Lịch sử đúng hạn -> 35 điểm.
// Tổng: 90/100.
func validAssessmentCommandInput() loandomain.LoanAssessmentInput {
	return loandomain.LoanAssessmentInput{
		MonthlyIncome: 10_000_000,
		Affordability: &loandomain.LoanAffordability{
			WithinIncomeLimit: true,
			TotalMonthlyDebt:  3_500_000,
			DTIBps:            3500,
			DTIStatus:         loandomain.DTIStatusPass,
		},
		InsuranceMonths:       24,
		InsuranceVerified:     true,
		RepaymentHistory:      loandomain.HistoryOnTime,
		HistoryVerified:       true,
		FinancialDataVerified: true,
		DocumentsVerified:     true,
	}
}

func validAssessLoanCommand() AssessLoanApplicationCommand {
	return AssessLoanApplicationCommand{
		ApplicationID: "loan-submit-001",
		ActorID:       "STAFF001",
	}
}

// 1. Kiểm tra cả ba nhánh quyết định.
func TestAssessLoanApplication_Decisions(t *testing.T) {
	tests := []struct {
		name          string
		modify        func(*loandomain.LoanAssessmentInput)
		wantDecision  loandomain.LoanAssessmentDecision
		wantStatus    loandomain.ApplicationStatus
		wantEvents    int
		wantLastEvent string
	}{
		{
			name:          "automatic approval",
			modify:        func(input *loandomain.LoanAssessmentInput) {},
			wantDecision:  loandomain.AssessmentAutoEligible,
			wantStatus:    loandomain.ApplicationStatusApproved,
			wantEvents:    2,
			wantLastEvent: loanApplicationApprovedEvent,
		},
		{
			name: "automatic rejection",
			modify: func(input *loandomain.LoanAssessmentInput) {
				input.RepaymentHistory = loandomain.HistoryOverdue
			},
			wantDecision:  loandomain.AssessmentRejected,
			wantStatus:    loandomain.ApplicationStatusRejected,
			wantEvents:    2,
			wantLastEvent: loanApplicationRejectedEvent,
		},
		{
			name: "manual review",
			modify: func(input *loandomain.LoanAssessmentInput) {
				input.DocumentsVerified = false
			},
			wantDecision:  loandomain.AssessmentManualReview,
			wantStatus:    loandomain.ApplicationStatusUnderReview,
			wantEvents:    1,
			wantLastEvent: loanApplicationAssessedEvent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newLoanForAssessmentTest(t)

			repo := &assessLoanRepoStub{
				app:      app,
				revision: 9,
			}

			authorizer := &assessLoanAuthorizerStub{}

			input := validAssessmentCommandInput()
			tt.modify(&input)

			provider := &assessLoanDataProviderStub{
				input: input,
			}

			handler := NewAssessLoanApplicationCommandHandler(
				repo,
				authorizer,
				provider,
			)

			result, err := handler.Handle(
				context.Background(),
				validAssessLoanCommand(),
			)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("expected assessed application")
			}

			if result.Status != tt.wantStatus {
				t.Fatalf(
					"expected status %s, got %s",
					tt.wantStatus,
					result.Status,
				)
			}

			if result.AssessmentDecision != tt.wantDecision {
				t.Fatalf(
					"expected decision %s, got %s",
					tt.wantDecision,
					result.AssessmentDecision,
				)
			}

			if result.AssessedAt == nil ||
				result.AssessedAt.IsZero() {
				t.Fatal("assessment timestamp is required")
			}

			// Không thay đổi aggregate ban đầu.
			if app.Status != loandomain.ApplicationStatusUnderReview {
				t.Fatal("original application status was modified")
			}

			if app.AssessedAt != nil {
				t.Fatal("original application was assessed")
			}

			// Kiểm tra Authorizer.
			if authorizer.calls != 1 ||
				authorizer.actorID != "STAFF001" {
				t.Fatal("incorrect authorization call")
			}

			// Kiểm tra DataProvider.
			if provider.calls != 1 ||
				provider.app != app {
				t.Fatal("incorrect assessment data provider call")
			}

			// Kiểm tra Repository.
			if repo.readCalls != 1 ||
				repo.appendCalls != 1 {
				t.Fatal("expected one read and one batch append")
			}

			if repo.savedID != app.Id ||
				repo.savedRev != 9 {
				t.Fatal("incorrect stream ID or revision")
			}

			if len(repo.savedEvents) != tt.wantEvents {
				t.Fatalf(
					"expected %d events, got %d",
					tt.wantEvents,
					len(repo.savedEvents),
				)
			}

			if repo.savedEvents[0].EventType !=
				loanApplicationAssessedEvent {
				t.Fatal("first event must be LoanApplicationAssessed")
			}

			lastEvent := repo.savedEvents[len(repo.savedEvents)-1]

			if lastEvent.EventType != tt.wantLastEvent {
				t.Fatalf(
					"expected last event %s, got %s",
					tt.wantLastEvent,
					lastEvent.EventType,
				)
			}

			// Kiểm tra dữ liệu chấm điểm trong event.
			data, err := json.Marshal(
				repo.savedEvents[0].Payload,
			)
			if err != nil {
				t.Fatal(err)
			}

			var payload struct {
				DTIScore       int                               `json:"dtiScore"`
				InsuranceScore int                               `json:"insuranceScore"`
				HistoryScore   int                               `json:"historyScore"`
				TotalScore     int                               `json:"totalScore"`
				Decision       loandomain.LoanAssessmentDecision `json:"decision"`
				Reasons        []string                          `json:"reasons"`
				PolicyVersion  string                            `json:"policyVersion"`
				AssessedAt     time.Time                         `json:"assessedAt"`
				UpdatedAt      time.Time                         `json:"updatedAt"`
			}

			if err := json.Unmarshal(data, &payload); err != nil {
				t.Fatal(err)
			}

			if payload.Decision != tt.wantDecision {
				t.Fatal("event contains incorrect decision")
			}

			if payload.PolicyVersion != loanAssessmentPolicyVersion {
				t.Fatal("missing assessment policy version")
			}

			if payload.TotalScore !=
				payload.DTIScore+
					payload.InsuranceScore+
					payload.HistoryScore {
				t.Fatal("assessment scores do not add up")
			}

			if payload.TotalScore != result.AssessmentScore {
				t.Fatal("event score differs from aggregate")
			}

			if !payload.AssessedAt.Equal(*result.AssessedAt) {
				t.Fatal("incorrect assessment timestamp in event")
			}

			// Auto approve/reject phải có decision event.
			if tt.wantEvents == 2 {
				decisionData, err := json.Marshal(
					repo.savedEvents[1].Payload,
				)
				if err != nil {
					t.Fatal(err)
				}

				var decisionPayload struct {
					DecidedAt time.Time `json:"decidedAt"`
					UpdatedAt time.Time `json:"updatedAt"`
				}

				if err := json.Unmarshal(
					decisionData,
					&decisionPayload,
				); err != nil {
					t.Fatal(err)
				}

				if decisionPayload.DecidedAt.IsZero() {
					t.Fatal("decision timestamp is required")
				}

				if tt.wantStatus ==
					loandomain.ApplicationStatusApproved {
					if result.ApprovedAt == nil ||
						!decisionPayload.DecidedAt.Equal(*result.ApprovedAt) {
						t.Fatal("incorrect approval timestamp")
					}
				}

				if tt.wantStatus ==
					loandomain.ApplicationStatusRejected {
					if result.RejectedAt == nil ||
						!decisionPayload.DecidedAt.Equal(*result.RejectedAt) {
						t.Fatal("incorrect rejection timestamp")
					}
				}
			}

			// Điểm của hồ sơ chuẩn phải là 90/100.
			if tt.name == "automatic approval" &&
				result.AssessmentScore != 90 {
				t.Fatalf(
					"expected score 90, got %d",
					result.AssessmentScore,
				)
			}
		})
	}
}

// 2. Kiểm tra dữ liệu và trạng thái không hợp lệ.
func TestAssessLoanApplication_InvalidRequests(t *testing.T) {
	tests := []struct {
		name   string
		modify func(
			*AssessLoanApplicationCommand,
			*loandomain.Application,
			*assessLoanRepoStub,
		)
	}{
		{
			name: "missing application ID",
			modify: func(
				cmd *AssessLoanApplicationCommand,
				app *loandomain.Application,
				repo *assessLoanRepoStub,
			) {
				cmd.ApplicationID = ""
			},
		},
		{
			name: "missing actor ID",
			modify: func(
				cmd *AssessLoanApplicationCommand,
				app *loandomain.Application,
				repo *assessLoanRepoStub,
			) {
				cmd.ActorID = ""
			},
		},
		{
			name: "application not found",
			modify: func(
				cmd *AssessLoanApplicationCommand,
				app *loandomain.Application,
				repo *assessLoanRepoStub,
			) {
				repo.app = nil
			},
		},
		{
			name: "application ID mismatch",
			modify: func(
				cmd *AssessLoanApplicationCommand,
				app *loandomain.Application,
				repo *assessLoanRepoStub,
			) {
				cmd.ApplicationID = "another-loan"
			},
		},
		{
			name: "application still draft",
			modify: func(
				cmd *AssessLoanApplicationCommand,
				app *loandomain.Application,
				repo *assessLoanRepoStub,
			) {
				app.Status = loandomain.ApplicationStatusDraft
			},
		},
		{
			name: "application still submitted",
			modify: func(
				cmd *AssessLoanApplicationCommand,
				app *loandomain.Application,
				repo *assessLoanRepoStub,
			) {
				app.Status = loandomain.ApplicationStatusSubmitted
			},
		},
		{
			name: "already assessed",
			modify: func(
				cmd *AssessLoanApplicationCommand,
				app *loandomain.Application,
				repo *assessLoanRepoStub,
			) {
				now := time.Now().UTC()
				app.AssessedAt = &now
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newLoanForAssessmentTest(t)

			repo := &assessLoanRepoStub{
				app:      app,
				revision: 9,
			}
			authorizer := &assessLoanAuthorizerStub{}
			provider := &assessLoanDataProviderStub{
				input: validAssessmentCommandInput(),
			}

			cmd := validAssessLoanCommand()
			tt.modify(&cmd, app, repo)

			handler := NewAssessLoanApplicationCommandHandler(
				repo,
				authorizer,
				provider,
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
				t.Fatal("invalid request must not append events")
			}

			if provider.calls != 0 {
				t.Fatal("invalid request must not build assessment")
			}
		})
	}
}

// 3. Người thực hiện không có quyền.
func TestAssessLoanApplication_Unauthorized(t *testing.T) {
	authErr := errors.New("assessment permission denied")

	repo := &assessLoanRepoStub{
		app: newLoanForAssessmentTest(t),
	}
	authorizer := &assessLoanAuthorizerStub{
		err: authErr,
	}
	provider := &assessLoanDataProviderStub{
		input: validAssessmentCommandInput(),
	}

	handler := NewAssessLoanApplicationCommandHandler(
		repo,
		authorizer,
		provider,
	)

	result, err := handler.Handle(
		context.Background(),
		validAssessLoanCommand(),
	)

	if !errors.Is(err, authErr) {
		t.Fatalf("expected authorization error, got %v", err)
	}

	if result != nil {
		t.Fatal("unauthorized request returned application")
	}

	if repo.readCalls != 0 || repo.appendCalls != 0 {
		t.Fatal("unauthorized request accessed repository")
	}

	if provider.calls != 0 {
		t.Fatal("unauthorized request called data provider")
	}
}

// 4. DataProvider không lấy được dữ liệu.
func TestAssessLoanApplication_DataProviderFailure(t *testing.T) {
	dataErr := errors.New("verified financial data unavailable")

	repo := &assessLoanRepoStub{
		app: newLoanForAssessmentTest(t),
	}
	authorizer := &assessLoanAuthorizerStub{}
	provider := &assessLoanDataProviderStub{
		err: dataErr,
	}

	handler := NewAssessLoanApplicationCommandHandler(
		repo,
		authorizer,
		provider,
	)

	result, err := handler.Handle(
		context.Background(),
		validAssessLoanCommand(),
	)

	if !errors.Is(err, dataErr) {
		t.Fatalf("expected data provider error, got %v", err)
	}

	if result != nil || repo.appendCalls != 0 {
		t.Fatal("data failure must not persist assessment")
	}
}

// 5. Domain từ chối dữ liệu thẩm định không hợp lệ.
func TestAssessLoanApplication_InvalidAssessmentInput(t *testing.T) {
	input := validAssessmentCommandInput()
	input.MonthlyIncome = 0

	repo := &assessLoanRepoStub{
		app: newLoanForAssessmentTest(t),
	}
	authorizer := &assessLoanAuthorizerStub{}
	provider := &assessLoanDataProviderStub{
		input: input,
	}

	handler := NewAssessLoanApplicationCommandHandler(
		repo,
		authorizer,
		provider,
	)

	result, err := handler.Handle(
		context.Background(),
		validAssessLoanCommand(),
	)

	if err == nil {
		t.Fatal("expected assessment input error")
	}

	if result != nil || repo.appendCalls != 0 {
		t.Fatal("invalid assessment must not persist events")
	}
}

// 6. EventStoreDB đọc hoặc ghi thất bại.
func TestAssessLoanApplication_RepositoryFailures(t *testing.T) {
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
			app := newLoanForAssessmentTest(t)

			repo := &assessLoanRepoStub{
				app:       app,
				revision:  9,
				readErr:   tt.readErr,
				appendErr: tt.appendErr,
			}
			authorizer := &assessLoanAuthorizerStub{}
			provider := &assessLoanDataProviderStub{
				input: validAssessmentCommandInput(),
			}

			handler := NewAssessLoanApplicationCommandHandler(
				repo,
				authorizer,
				provider,
			)

			result, err := handler.Handle(
				context.Background(),
				validAssessLoanCommand(),
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
				t.Fatal("failed persistence returned application")
			}

			if app.Status != loandomain.ApplicationStatusUnderReview ||
				app.AssessedAt != nil {
				t.Fatal("repository failure modified original application")
			}

			if tt.readErr != nil && repo.appendCalls != 0 {
				t.Fatal("read failure must not append events")
			}

			if tt.appendErr != nil && repo.appendCalls != 1 {
				t.Fatal("expected one batch append attempt")
			}
		})
	}
}

// 7. Thiếu dependency.
func TestAssessLoanApplication_MissingDependency(t *testing.T) {
	repo := &assessLoanRepoStub{}
	authorizer := &assessLoanAuthorizerStub{}
	provider := &assessLoanDataProviderStub{}

	tests := []struct {
		name    string
		handler *AssessLoanApplicationCommandHandler
	}{
		{
			name:    "nil handler",
			handler: nil,
		},
		{
			name: "nil repository",
			handler: NewAssessLoanApplicationCommandHandler(
				nil,
				authorizer,
				provider,
			),
		},
		{
			name: "nil authorizer",
			handler: NewAssessLoanApplicationCommandHandler(
				repo,
				nil,
				provider,
			),
		},
		{
			name: "nil data provider",
			handler: NewAssessLoanApplicationCommandHandler(
				repo,
				authorizer,
				nil,
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.handler.Handle(
				context.Background(),
				validAssessLoanCommand(),
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
