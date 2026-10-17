package loan

import (
	"errors"
	"testing"
	"time"
)

// Tạo hồ sơ đã được phân luồng thẩm định thủ công.
func newManualReviewTestApplication(t *testing.T) *Application {
	t.Helper()

	app := newUnderReviewTestApplication(t)
	input := validAssessmentTestInput()

	// DTI = 45%, cần thẩm định thủ công.
	input.Affordability.TotalMonthlyDebt = 4_500_000

	result, err := app.ApplyAssessment(input)
	if err != nil {
		t.Fatalf("failed to assess application: %v", err)
	}

	if result.Decision != AssessmentManualReview {
		t.Fatalf("expected manual_review, got %s", result.Decision)
	}

	return app
}

// Kiểm tra nhân viên phê duyệt hoặc từ chối hồ sơ.
func TestApplyManualDecision_Success(t *testing.T) {
	tests := []struct {
		name     string
		decision ApplicationStatus
	}{
		{
			name:     "manual approval",
			decision: ApplicationStatusApproved,
		},
		{
			name:     "manual rejection",
			decision: ApplicationStatusRejected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newManualReviewTestApplication(t)

			before := time.Now().UTC()

			err := app.ApplyManualDecision(
				"STAFF001",
				tt.decision,
				"Reviewed customer documents",
			)

			after := time.Now().UTC()

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if app.Status != tt.decision {
				t.Errorf(
					"expected %s, got %s",
					tt.decision, app.Status,
				)
			}

			if app.ManualReviewerId != "STAFF001" {
				t.Error("reviewer ID was not saved")
			}

			if app.ManualDecisionReason !=
				"Reviewed customer documents" {
				t.Error("decision reason was not saved")
			}

			if app.ManualDecidedAt == nil {
				t.Fatal("manual decision timestamp is required")
			}

			if app.ManualDecidedAt.Before(before) ||
				app.ManualDecidedAt.After(after) {
				t.Error("invalid manual decision timestamp")
			}

			if !app.UpdatedAt.Equal(*app.ManualDecidedAt) {
				t.Error("updatedAt must equal manualDecidedAt")
			}

			// Kết quả phân luồng ban đầu vẫn được giữ lại.
			if app.AssessmentDecision != AssessmentManualReview {
				t.Error("original assessment decision must remain")
			}

			switch tt.decision {
			case ApplicationStatusApproved:
				if app.ApprovedAt == nil {
					t.Error("approvedAt must not be nil")
				}

				if app.RejectedAt != nil {
					t.Error("rejectedAt must remain nil")
				}

			case ApplicationStatusRejected:
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

// Kiểm tra thông tin quyết định không hợp lệ.
func TestApplyManualDecision_InvalidInput(t *testing.T) {
	tests := []struct {
		name       string
		reviewerId string
		decision   ApplicationStatus
		reason     string
	}{
		{
			name:       "empty reviewer ID",
			reviewerId: "",
			decision:   ApplicationStatusApproved,
			reason:     "Reviewed",
		},
		{
			name:       "blank reviewer ID",
			reviewerId: "   ",
			decision:   ApplicationStatusApproved,
			reason:     "Reviewed",
		},
		{
			name:       "empty reason",
			reviewerId: "STAFF001",
			decision:   ApplicationStatusApproved,
			reason:     "",
		},
		{
			name:       "blank reason",
			reviewerId: "STAFF001",
			decision:   ApplicationStatusApproved,
			reason:     "   ",
		},
		{
			name:       "invalid decision",
			reviewerId: "STAFF001",
			decision:   ApplicationStatusSubmitted,
			reason:     "Reviewed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newManualReviewTestApplication(t)
			previousUpdatedAt := app.UpdatedAt

			err := app.ApplyManualDecision(
				tt.reviewerId,
				tt.decision,
				tt.reason,
			)

			if err == nil {
				t.Fatal("expected validation error")
			}

			if app.Status != ApplicationStatusUnderReview {
				t.Error("status must remain under_review")
			}

			if app.ManualDecidedAt != nil {
				t.Error("manualDecidedAt must remain nil")
			}

			if app.ManualReviewerId != "" ||
				app.ManualDecisionReason != "" {
				t.Error("invalid decision must not save metadata")
			}

			if !app.UpdatedAt.Equal(previousUpdatedAt) {
				t.Error("updatedAt must not change")
			}
		})
	}
}

// Kiểm tra không thể quyết định khi sai trạng thái.
func TestApplyManualDecision_InvalidStatus(t *testing.T) {
	statuses := []ApplicationStatus{
		ApplicationStatusDraft,
		ApplicationStatusSubmitted,
		ApplicationStatusApproved,
		ApplicationStatusRejected,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			app := newManualReviewTestApplication(t)
			app.Status = status

			err := app.ApplyManualDecision(
				"STAFF001",
				ApplicationStatusApproved,
				"Reviewed",
			)

			if !errors.Is(err, ErrInvalidApplicationStatus) {
				t.Errorf(
					"expected ErrInvalidApplicationStatus, got %v",
					err,
				)
			}

			if app.Status != status {
				t.Error("status must not change")
			}

			if app.ManualDecidedAt != nil {
				t.Error("manualDecidedAt must remain nil")
			}
		})
	}
}

// Kiểm tra hồ sơ chưa được phân luồng thẩm định.
func TestApplyManualDecision_NotAssessed(t *testing.T) {
	app := newUnderReviewTestApplication(t)

	err := app.ApplyManualDecision(
		"STAFF001",
		ApplicationStatusApproved,
		"Reviewed",
	)

	if err == nil {
		t.Fatal("expected error for application not assessed")
	}

	if app.Status != ApplicationStatusUnderReview {
		t.Error("status must remain under_review")
	}

	if app.ApprovedAt != nil {
		t.Error("approvedAt must remain nil")
	}
}

// Kiểm tra không thể quyết định hai lần.
func TestApplyManualDecision_Twice(t *testing.T) {
	app := newManualReviewTestApplication(t)

	err := app.ApplyManualDecision(
		"STAFF001",
		ApplicationStatusApproved,
		"Approved after review",
	)
	if err != nil {
		t.Fatalf("first decision failed: %v", err)
	}

	firstDecisionAt := *app.ManualDecidedAt
	firstUpdatedAt := app.UpdatedAt

	err = app.ApplyManualDecision(
		"STAFF002",
		ApplicationStatusRejected,
		"Attempt to change decision",
	)

	if !errors.Is(err, ErrInvalidApplicationStatus) {
		t.Errorf("expected status error, got %v", err)
	}

	if app.Status != ApplicationStatusApproved {
		t.Error("status must remain approved")
	}

	if app.ManualReviewerId != "STAFF001" {
		t.Error("reviewer ID must not change")
	}

	if !app.ManualDecidedAt.Equal(firstDecisionAt) {
		t.Error("decision timestamp must not change")
	}

	if !app.UpdatedAt.Equal(firstUpdatedAt) {
		t.Error("updatedAt must not change")
	}
}

// Kiểm tra hồ sơ nil.
func TestApplyManualDecision_NilApplication(t *testing.T) {
	var app *Application

	err := app.ApplyManualDecision(
		"STAFF001",
		ApplicationStatusApproved,
		"Reviewed",
	)

	if err == nil {
		t.Fatal("expected error for nil application")
	}
}
