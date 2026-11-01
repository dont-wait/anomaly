package eventstore

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
)

// Chuyển payload thành JSON phục vụ kiểm thử.
func loanReplayJSON(t *testing.T, payload any) []byte {
	t.Helper()

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	return data
}

// Áp dụng một sự kiện và yêu cầu không có lỗi.
func mustReplayLoanEvent(
	t *testing.T,
	app *loandomain.Application,
	eventType string,
	payload any,
) {
	t.Helper()

	err := applyLoanEvent(
		app,
		eventType,
		loanReplayJSON(t, payload),
	)
	if err != nil {
		t.Fatalf("replay %s failed: %v", eventType, err)
	}
}

// Tạo hồ sơ bằng cách replay các sự kiện:
// Created -> DocumentAdded -> Submitted -> ReviewStarted.
func prepareLoanReviewReplay(
	t *testing.T,
) (*loandomain.Application, time.Time) {
	t.Helper()

	baseTime := time.Date(
		2026, 10, 14, 8, 0, 0, 0, time.UTC,
	)

	draft, err := loandomain.NewDraftApplication(
		"loan-001",
		"LA001",
		"customer-001",
		"package-001",
		"account-001",
		10_000_000,
		12,
	)
	if err != nil {
		t.Fatalf("failed to create draft: %v", err)
	}

	draft.CreatedAt = baseTime
	draft.UpdatedAt = baseTime

	app := &loandomain.Application{}

	mustReplayLoanEvent(
		t,
		app,
		EventLoanApplicationCreated,
		loanApplicationCreatedPayload{
			Application: draft,
		},
	)

	documentTypes := []string{
		"IDENTITY_FRONT",
		"IDENTITY_BACK",
		"VNEID_PERSONAL",
		"VNEID_RESIDENCE",
		"VSSID_PARTICIPATION",
		"VSSID_DETAIL",
		"SALARY_STATEMENT_3_MONTHS",
	}

	for i, documentType := range documentTypes {
		documentID := fmt.Sprintf("doc-%d", i+1)
		uploadedAt := baseTime.Add(
			time.Duration(i+1) * time.Minute,
		)

		document := loandomain.ApplicationDocument{
			DocumentId:         documentID,
			Type:               documentType,
			StorageKey:         "loans/" + documentID,
			MIMEType:           "application/pdf",
			SizeBytes:          1024,
			SHA256:             "test-sha256-" + documentID,
			VerificationStatus: "pending",
			UploadedAt:         uploadedAt,
		}

		mustReplayLoanEvent(
			t,
			app,
			EventLoanDocumentAdded,
			loanDocumentAddedPayload{
				Document:  document,
				UpdatedAt: uploadedAt,
			},
		)
	}

	submittedAt := baseTime.Add(10 * time.Minute)

	mustReplayLoanEvent(
		t,
		app,
		EventLoanApplicationSubmitted,
		loanApplicationSubmittedPayload{
			SubmittedAt: submittedAt,
			UpdatedAt:   submittedAt,
		},
	)

	reviewStartedAt := baseTime.Add(11 * time.Minute)

	mustReplayLoanEvent(
		t,
		app,
		EventLoanReviewStarted,
		loanReviewStartedPayload{
			ReviewStartedAt: reviewStartedAt,
			UpdatedAt:       reviewStartedAt,
		},
	)

	return app, baseTime
}

// Kiểm tra replay toàn bộ luồng phê duyệt tự động.
func TestApplyLoanEvent_ReplayAutoApproved(t *testing.T) {
	app, baseTime := prepareLoanReviewReplay(t)

	if app.Status != loandomain.ApplicationStatusUnderReview {
		t.Fatalf("expected under_review, got %s", app.Status)
	}

	if len(app.Documents) != 7 {
		t.Fatalf(
			"expected 7 documents, got %d",
			len(app.Documents),
		)
	}

	if app.SubmittedAt == nil || app.ReviewStartedAt == nil {
		t.Fatal("submission and review timestamps are required")
	}

	assessedAt := baseTime.Add(12 * time.Minute)

	mustReplayLoanEvent(
		t,
		app,
		EventLoanApplicationAssessed,
		loanApplicationAssessedPayload{
			DTIScore:       30,
			InsuranceScore: 25,
			HistoryScore:   35,
			TotalScore:     90,
			Decision:       loandomain.AssessmentAutoEligible,
			Reasons:        []string{},
			PolicyVersion:  "loan-policy-v1",
			AssessedAt:     assessedAt,
			UpdatedAt:      assessedAt,
		},
	)

	if app.AssessmentScore != 90 {
		t.Errorf(
			"expected assessment score 90, got %d",
			app.AssessmentScore,
		)
	}

	if app.AssessmentDecision != loandomain.AssessmentAutoEligible {
		t.Error("incorrect assessment decision")
	}

	if app.AssessedAt == nil || !app.AssessedAt.Equal(assessedAt) {
		t.Fatal("incorrect assessment timestamp")
	}

	// Chấm điểm chưa đồng nghĩa đã phê duyệt.
	if app.Status != loandomain.ApplicationStatusUnderReview {
		t.Error("assessment must not directly finalize application")
	}

	approvedAt := baseTime.Add(13 * time.Minute)

	mustReplayLoanEvent(
		t,
		app,
		EventLoanApplicationApproved,
		loanDecisionPayload{
			DecidedAt: approvedAt,
			UpdatedAt: approvedAt,
		},
	)

	if app.Status != loandomain.ApplicationStatusApproved {
		t.Errorf("expected approved, got %s", app.Status)
	}

	if app.ApprovedAt == nil || !app.ApprovedAt.Equal(approvedAt) {
		t.Error("incorrect approval timestamp")
	}

	if app.RejectedAt != nil {
		t.Error("rejectedAt must remain nil")
	}

	if app.ManualReviewerId != "" {
		t.Error("automatic approval must not have manual reviewer")
	}
}

// Kiểm tra replay phê duyệt và từ chối thủ công.
func TestApplyLoanEvent_ReplayManualDecision(t *testing.T) {
	tests := []struct {
		name           string
		eventType      string
		expectedStatus loandomain.ApplicationStatus
	}{
		{
			name:           "manual approval",
			eventType:      EventLoanApplicationApproved,
			expectedStatus: loandomain.ApplicationStatusApproved,
		},
		{
			name:           "manual rejection",
			eventType:      EventLoanApplicationRejected,
			expectedStatus: loandomain.ApplicationStatusRejected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, baseTime := prepareLoanReviewReplay(t)

			assessedAt := baseTime.Add(12 * time.Minute)

			mustReplayLoanEvent(
				t,
				app,
				EventLoanApplicationAssessed,
				loanApplicationAssessedPayload{
					DTIScore:       15,
					InsuranceScore: 25,
					HistoryScore:   35,
					TotalScore:     75,
					Decision:       loandomain.AssessmentManualReview,
					Reasons: []string{
						"DTI requires manual review",
					},
					PolicyVersion: "loan-policy-v1",
					AssessedAt:    assessedAt,
					UpdatedAt:     assessedAt,
				},
			)

			decidedAt := baseTime.Add(15 * time.Minute)

			mustReplayLoanEvent(
				t,
				app,
				tt.eventType,
				loanDecisionPayload{
					DecidedAt:            decidedAt,
					UpdatedAt:            decidedAt,
					ManualReviewerId:     "STAFF001",
					ManualDecisionReason: "Reviewed customer documents",
					ManualDecidedAt:      &decidedAt,
				},
			)

			if app.Status != tt.expectedStatus {
				t.Errorf(
					"expected %s, got %s",
					tt.expectedStatus,
					app.Status,
				)
			}

			if app.AssessmentScore != 75 {
				t.Errorf(
					"expected score 75, got %d",
					app.AssessmentScore,
				)
			}

			if app.AssessmentDecision != loandomain.AssessmentManualReview {
				t.Error("original assessment decision must remain")
			}

			if app.ManualReviewerId != "STAFF001" {
				t.Error("manual reviewer was not restored")
			}

			if app.ManualDecisionReason != "Reviewed customer documents" {
				t.Error("manual decision reason was not restored")
			}

			if app.ManualDecidedAt == nil ||
				!app.ManualDecidedAt.Equal(decidedAt) {
				t.Error("incorrect manual decision timestamp")
			}

			switch tt.expectedStatus {
			case loandomain.ApplicationStatusApproved:
				if app.ApprovedAt == nil {
					t.Error("approvedAt must not be nil")
				}
				if app.RejectedAt != nil {
					t.Error("rejectedAt must remain nil")
				}

			case loandomain.ApplicationStatusRejected:
				if app.RejectedAt == nil {
					t.Error("rejectedAt must not be nil")
				}
				if app.ApprovedAt != nil {
					t.Error("approvedAt must remain nil")
				}
			}
		})
	}
}

// Kiểm tra replay hồ sơ bị từ chối tự động.
func TestApplyLoanEvent_ReplayAutoRejected(t *testing.T) {
	app, baseTime := prepareLoanReviewReplay(t)

	assessedAt := baseTime.Add(12 * time.Minute)

	mustReplayLoanEvent(
		t,
		app,
		EventLoanApplicationAssessed,
		loanApplicationAssessedPayload{
			DTIScore:       0,
			InsuranceScore: 25,
			HistoryScore:   35,
			TotalScore:     60,
			Decision:       loandomain.AssessmentRejected,
			Reasons: []string{
				"DTI exceeds 50 percent",
			},
			PolicyVersion: "loan-policy-v1",
			AssessedAt:    assessedAt,
			UpdatedAt:     assessedAt,
		},
	)

	rejectedAt := baseTime.Add(13 * time.Minute)

	mustReplayLoanEvent(
		t,
		app,
		EventLoanApplicationRejected,
		loanDecisionPayload{
			DecidedAt: rejectedAt,
			UpdatedAt: rejectedAt,
		},
	)

	if app.Status != loandomain.ApplicationStatusRejected {
		t.Errorf("expected rejected, got %s", app.Status)
	}

	if app.RejectedAt == nil || !app.RejectedAt.Equal(rejectedAt) {
		t.Error("incorrect rejection timestamp")
	}

	if len(app.AssessmentReasons) != 1 {
		t.Error("rejection reason must be restored")
	}

	if app.ManualDecidedAt != nil {
		t.Error("automatic rejection must not have manual timestamp")
	}
}

// Kiểm tra các sự kiện không hợp lệ.
func TestApplyLoanEvent_InvalidEvents(t *testing.T) {
	t.Run("event before creation", func(t *testing.T) {
		app := &loandomain.Application{}

		err := applyLoanEvent(
			app,
			EventLoanApplicationSubmitted,
			loanReplayJSON(t, loanApplicationSubmittedPayload{}),
		)

		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("duplicate creation", func(t *testing.T) {
		app, _ := prepareLoanReviewReplay(t)

		err := applyLoanEvent(
			app,
			EventLoanApplicationCreated,
			loanReplayJSON(t, loanApplicationCreatedPayload{
				Application: app,
			}),
		)

		if err == nil {
			t.Fatal("expected duplicate creation error")
		}
	})

	t.Run("invalid creation payload", func(t *testing.T) {
		app := &loandomain.Application{}

		err := applyLoanEvent(
			app,
			EventLoanApplicationCreated,
			[]byte(`{}`),
		)

		if err == nil {
			t.Fatal("expected invalid creation error")
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		app := &loandomain.Application{}

		err := applyLoanEvent(
			app,
			EventLoanApplicationCreated,
			[]byte(`{invalid-json`),
		)

		if err == nil {
			t.Fatal("expected JSON error")
		}
	})

	t.Run("approve before assessment", func(t *testing.T) {
		app, _ := prepareLoanReviewReplay(t)

		err := applyLoanEvent(
			app,
			EventLoanApplicationApproved,
			loanReplayJSON(t, loanDecisionPayload{}),
		)

		if err == nil {
			t.Fatal("expected error before assessment")
		}
	})

	t.Run("invalid assessment decision", func(t *testing.T) {
		app, _ := prepareLoanReviewReplay(t)
		assessedAt := time.Now().UTC()
		err := applyLoanEvent(
			app,
			EventLoanApplicationAssessed,
			loanReplayJSON(t, loanApplicationAssessedPayload{
				DTIScore:       30,
				InsuranceScore: 25,
				HistoryScore:   35,
				TotalScore:     90,
				Decision:       "invalid",
				Reasons:        []string{},
				PolicyVersion:  "loan-policy-v1",
				AssessedAt:     assessedAt,
				UpdatedAt:      assessedAt,
			}),
		)

		if err == nil {
			t.Fatal("expected invalid decision error")
		}
	})

	t.Run("unknown event type", func(t *testing.T) {
		app, _ := prepareLoanReviewReplay(t)

		err := applyLoanEvent(
			app,
			"UnknownLoanEvent",
			[]byte(`{}`),
		)

		if err == nil {
			t.Fatal("expected unknown event error")
		}
	})

	t.Run("nil application", func(t *testing.T) {
		var app *loandomain.Application

		err := applyLoanEvent(
			app,
			EventLoanApplicationCreated,
			[]byte(`{}`),
		)

		if err == nil {
			t.Fatal("expected nil application error")
		}
	})
}

// Kiểm tra tên stream của từng hồ sơ.
func TestLoanStreamName(t *testing.T) {
	got := loanStreamName("LA001")
	expected := "loan-application-LA001"

	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

// Tạo một hồ sơ draft thông qua Event Replay.
func newLoanDraftReplayForValidation(
	t *testing.T,
) *loandomain.Application {
	t.Helper()

	draft, err := loandomain.NewDraftApplication(
		"loan-validation-001",
		"LA-VALIDATION-001",
		"customer-001",
		"package-001",
		"account-001",
		10_000_000,
		12,
	)
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}

	app := &loandomain.Application{}

	mustReplayLoanEvent(
		t,
		app,
		EventLoanApplicationCreated,
		loanApplicationCreatedPayload{
			Application: draft,
		},
	)

	return app
}

// 1. Không cho submit nếu thiếu chứng từ.
func TestApplyLoanEvent_RejectIncompleteSubmission(t *testing.T) {
	app := newLoanDraftReplayForValidation(t)

	submittedAt := app.CreatedAt.Add(time.Minute)

	err := applyLoanEvent(
		app,
		EventLoanApplicationSubmitted,
		loanReplayJSON(t, loanApplicationSubmittedPayload{
			SubmittedAt: submittedAt,
			UpdatedAt:   submittedAt,
		}),
	)

	if err == nil {
		t.Fatal("expected missing documents error")
	}

	if app.Status != loandomain.ApplicationStatusDraft {
		t.Fatalf("status changed unexpectedly: %s", app.Status)
	}

	if app.SubmittedAt != nil {
		t.Fatal("submittedAt must remain nil")
	}
}

// 2. Không cho thêm chứng từ trùng ID.
func TestApplyLoanEvent_RejectDuplicateDocument(t *testing.T) {
	app := newLoanDraftReplayForValidation(t)

	uploadedAt := app.CreatedAt.Add(time.Minute)

	payload := loanDocumentAddedPayload{
		Document: loandomain.ApplicationDocument{
			DocumentId:         "doc-001",
			Type:               "IDENTITY_FRONT",
			StorageKey:         "loans/doc-001",
			MIMEType:           "image/jpeg",
			SizeBytes:          2048,
			SHA256:             "test-sha256",
			VerificationStatus: "pending",
			UploadedAt:         uploadedAt,
		},
		UpdatedAt: uploadedAt,
	}

	mustReplayLoanEvent(
		t,
		app,
		EventLoanDocumentAdded,
		payload,
	)

	// Cố gắng thêm lại cùng DocumentId.
	err := applyLoanEvent(
		app,
		EventLoanDocumentAdded,
		loanReplayJSON(t, payload),
	)

	if err == nil {
		t.Fatal("expected duplicate document error")
	}

	if len(app.Documents) != 1 {
		t.Fatalf(
			"expected 1 document, got %d",
			len(app.Documents),
		)
	}
}

// 3. Kiểm tra kết quả chấm điểm sai nghiệp vụ.
func TestApplyLoanEvent_RejectInvalidAssessmentScores(t *testing.T) {
	tests := []struct {
		name           string
		dtiScore       int
		insuranceScore int
		historyScore   int
		totalScore     int
		decision       loandomain.LoanAssessmentDecision
		reasons        []string
	}{
		{
			name:           "incorrect total score",
			dtiScore:       30,
			insuranceScore: 25,
			historyScore:   35,
			totalScore:     95,
			decision:       loandomain.AssessmentAutoEligible,
		},
		{
			name:           "invalid component score",
			dtiScore:       99,
			insuranceScore: 25,
			historyScore:   35,
			totalScore:     159,
			decision:       loandomain.AssessmentManualReview,
		},
		{
			name:           "automatic approval below 80 points",
			dtiScore:       15,
			insuranceScore: 25,
			historyScore:   35,
			totalScore:     75,
			decision:       loandomain.AssessmentAutoEligible,
		},
		{
			name:           "automatic approval with weak insurance",
			dtiScore:       40,
			insuranceScore: 10,
			historyScore:   35,
			totalScore:     85,
			decision:       loandomain.AssessmentAutoEligible,
		},
		{
			name:           "rejection without reason",
			dtiScore:       0,
			insuranceScore: 25,
			historyScore:   35,
			totalScore:     60,
			decision:       loandomain.AssessmentRejected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, baseTime := prepareLoanReviewReplay(t)

			assessedAt := baseTime.Add(12 * time.Minute)

			payload := loanApplicationAssessedPayload{
				DTIScore:       tt.dtiScore,
				InsuranceScore: tt.insuranceScore,
				HistoryScore:   tt.historyScore,
				TotalScore:     tt.totalScore,
				Decision:       tt.decision,
				Reasons:        tt.reasons,
				PolicyVersion:  "loan-policy-v1",
				AssessedAt:     assessedAt,
				UpdatedAt:      assessedAt,
			}

			err := applyLoanEvent(
				app,
				EventLoanApplicationAssessed,
				loanReplayJSON(t, payload),
			)

			if err == nil {
				t.Fatal("expected invalid assessment error")
			}

			if app.AssessedAt != nil {
				t.Fatal("invalid assessment must not be saved")
			}

			if app.Status != loandomain.ApplicationStatusUnderReview {
				t.Fatal("invalid assessment changed status")
			}
		})
	}
}

// 4. Quyết định thủ công bắt buộc có reviewer và reason.
func TestApplyLoanEvent_RejectManualDecisionWithoutReviewer(
	t *testing.T,
) {
	eventTypes := []string{
		EventLoanApplicationApproved,
		EventLoanApplicationRejected,
	}

	for _, eventType := range eventTypes {
		t.Run(eventType, func(t *testing.T) {
			app, baseTime := prepareLoanReviewReplay(t)

			assessedAt := baseTime.Add(12 * time.Minute)

			mustReplayLoanEvent(
				t,
				app,
				EventLoanApplicationAssessed,
				loanApplicationAssessedPayload{
					DTIScore:       15,
					InsuranceScore: 25,
					HistoryScore:   35,
					TotalScore:     75,
					Decision:       loandomain.AssessmentManualReview,
					Reasons: []string{
						"DTI requires manual review",
					},
					PolicyVersion: "loan-policy-v1",
					AssessedAt:    assessedAt,
					UpdatedAt:     assessedAt,
				},
			)

			decidedAt := baseTime.Add(13 * time.Minute)

			err := applyLoanEvent(
				app,
				eventType,
				loanReplayJSON(t, loanDecisionPayload{
					DecidedAt: decidedAt,
					UpdatedAt: decidedAt,
					// Thiếu reviewer, reason, manual timestamp.
				}),
			)

			if err == nil {
				t.Fatal("expected missing reviewer error")
			}

			if app.Status != loandomain.ApplicationStatusUnderReview {
				t.Fatal("invalid manual decision changed status")
			}

			if app.ApprovedAt != nil || app.RejectedAt != nil {
				t.Fatal("invalid manual decision was finalized")
			}
		})
	}
}

// 5. Không được từ chối tự động nếu kết quả là auto eligible.
func TestApplyLoanEvent_RejectContradictoryDecision(t *testing.T) {
	app, baseTime := prepareLoanReviewReplay(t)

	assessedAt := baseTime.Add(12 * time.Minute)

	mustReplayLoanEvent(
		t,
		app,
		EventLoanApplicationAssessed,
		loanApplicationAssessedPayload{
			DTIScore:       30,
			InsuranceScore: 25,
			HistoryScore:   35,
			TotalScore:     90,
			Decision:       loandomain.AssessmentAutoEligible,
			PolicyVersion:  "loan-policy-v1",
			AssessedAt:     assessedAt,
			UpdatedAt:      assessedAt,
		},
	)

	decidedAt := baseTime.Add(13 * time.Minute)

	// Kết quả đủ điều kiện phê duyệt,
	// nhưng event lại yêu cầu từ chối.
	err := applyLoanEvent(
		app,
		EventLoanApplicationRejected,
		loanReplayJSON(t, loanDecisionPayload{
			DecidedAt: decidedAt,
			UpdatedAt: decidedAt,
		}),
	)

	if err == nil {
		t.Fatal("expected contradictory decision error")
	}

	if app.Status != loandomain.ApplicationStatusUnderReview {
		t.Fatal("contradictory decision changed status")
	}

	if app.RejectedAt != nil {
		t.Fatal("invalid rejection was applied")
	}
}

// 6. Không cho chấm điểm lần thứ hai.
func TestApplyLoanEvent_RejectRepeatedAssessment(t *testing.T) {
	app, baseTime := prepareLoanReviewReplay(t)

	assessedAt := baseTime.Add(12 * time.Minute)

	payload := loanApplicationAssessedPayload{
		DTIScore:       30,
		InsuranceScore: 25,
		HistoryScore:   35,
		TotalScore:     90,
		Decision:       loandomain.AssessmentAutoEligible,
		PolicyVersion:  "loan-policy-v1",
		AssessedAt:     assessedAt,
		UpdatedAt:      assessedAt,
	}

	mustReplayLoanEvent(
		t,
		app,
		EventLoanApplicationAssessed,
		payload,
	)

	// Gửi lại event đánh giá lần thứ hai.
	err := applyLoanEvent(
		app,
		EventLoanApplicationAssessed,
		loanReplayJSON(t, payload),
	)

	if err == nil {
		t.Fatal("expected repeated assessment error")
	}

	if app.AssessmentScore != 90 {
		t.Fatal("original assessment score was modified")
	}
}

func TestApplyLoanEvent_ReplayPreparedDisbursement(t *testing.T) {
	app, baseTime := prepareLoanReviewReplay(t)

	assessedAt := baseTime.Add(12 * time.Minute)

	mustReplayLoanEvent(
		t,
		app,
		EventLoanApplicationAssessed,
		loanApplicationAssessedPayload{
			DTIScore:       30,
			InsuranceScore: 25,
			HistoryScore:   35,
			TotalScore:     90,
			Decision:       loandomain.AssessmentAutoEligible,
			Reasons:        []string{},
			PolicyVersion:  "loan-policy-v1",
			AssessedAt:     assessedAt,
			UpdatedAt:      assessedAt,
		},
	)

	approvedAt := baseTime.Add(13 * time.Minute)

	mustReplayLoanEvent(
		t,
		app,
		EventLoanApplicationApproved,
		loanDecisionPayload{
			DecidedAt: approvedAt,
			UpdatedAt: approvedAt,
		},
	)

	const fixedRate int64 = 1540
	preparedAt := approvedAt.Add(time.Minute)

	// Replay event chốt điều khoản trước giải ngân.
	mustReplayLoanEvent(
		t,
		app,
		EventLoanDisbursementPrepared,
		loanDisbursementPreparedPayload{
			PreparedAt:    preparedAt,
			AnnualRateBps: fixedRate,
			UpdatedAt:     preparedAt,
		},
	)

	if app.DisbursementPreparedAt == nil ||
		!app.DisbursementPreparedAt.Equal(preparedAt) {
		t.Fatal("prepared timestamp was not restored")
	}

	if app.PreparedAnnualRateBps != fixedRate {
		t.Fatal("prepared interest rate was not restored")
	}

	if app.Status != loandomain.ApplicationStatusApproved {
		t.Fatal("preparation must not disburse the loan")
	}

	// Tiền được cộng sau khi điều khoản đã được chốt.
	creditedAt := preparedAt.Add(time.Second)

	schedule, err := loandomain.CalculateRepaymentSchedule(
		app.RequestedAmount,
		fixedRate,
		app.TermMonths,
		creditedAt,
	)
	if err != nil {
		t.Fatal(err)
	}

	mustReplayLoanEvent(
		t,
		app,
		EventLoanDisbursed,
		loanDisbursedPayload{
			DisbursedAt:       creditedAt,
			AnnualRateBps:     fixedRate,
			RepaymentSchedule: schedule,
			UpdatedAt:         creditedAt,
		},
	)

	if app.Status != loandomain.ApplicationStatusDisbursed {
		t.Fatal("loan was not disbursed")
	}

	if app.AnnualRateBps != fixedRate {
		t.Fatal("disbursement interest rate mismatch")
	}

	if app.DisbursedAt == nil ||
		!app.DisbursedAt.Equal(creditedAt) {
		t.Fatal("incorrect disbursement timestamp")
	}

	if len(app.RepaymentSchedule) != app.TermMonths {
		t.Fatal("incorrect number of repayment installments")
	}

	last := app.RepaymentSchedule[len(app.RepaymentSchedule)-1]
	if last.RemainingBalance != 0 {
		t.Fatal("remaining balance must be zero after final installment")
	}
}

func TestReplayLoanDocumentReviewed(t *testing.T) {
	app, base := prepareLoanReviewReplay(t)
	documentID := app.Documents[0].DocumentId
	mustReplayLoanEvent(t, app, EventLoanDocumentReviewed, map[string]any{
		"documentId": documentID, "decision": "rejected", "reason": "Ảnh mờ",
		"reviewerId": "admin", "reviewedAt": base.Add(time.Hour),
	})
	document := app.Documents[0]
	if document.VerificationStatus != "rejected" || document.RejectionReason != "Ảnh mờ" || document.ReviewedBy != "admin" {
		t.Fatalf("document = %#v", document)
	}
	if err := applyLoanEvent(app, EventLoanDocumentReviewed, loanReplayJSON(t, map[string]any{
		"documentId": "missing", "decision": "verified", "reviewerId": "admin", "reviewedAt": base,
	})); err == nil {
		t.Fatal("expected an error for an unknown document")
	}
}
