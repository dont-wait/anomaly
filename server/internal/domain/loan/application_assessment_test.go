package loan

import (
	"errors"
	"testing"
)

// Tạo hồ sơ đã chuyển sang trạng thái under_review.
func newUnderReviewTestApplication(t *testing.T) *Application {
	t.Helper()

	app := newSubmittedTestApplication(t)

	if err := app.StartReview(); err != nil {
		t.Fatalf("failed to start review: %v", err)
	}

	return app
}

// Kiểm tra hồ sơ được phê duyệt tự động.
func TestApplyAssessment_AutoApproved(t *testing.T) {
	app := newUnderReviewTestApplication(t)
	input := validAssessmentTestInput()

	result, err := app.ApplyAssessment(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Decision != AssessmentAutoEligible {
		t.Errorf("unexpected decision: %s", result.Decision)
	}

	if result.TotalScore != 90 {
		t.Errorf("expected score 90, got %d", result.TotalScore)
	}

	if app.Status != ApplicationStatusApproved {
		t.Errorf("expected approved, got %s", app.Status)
	}

	if app.AssessmentScore != 90 {
		t.Errorf("expected saved score 90, got %d", app.AssessmentScore)
	}

	if app.AssessmentDecision != AssessmentAutoEligible {
		t.Error("assessment decision was not saved")
	}

	if app.AssessedAt == nil || app.ApprovedAt == nil {
		t.Fatal("assessment and approval timestamps are required")
	}

	if !app.AssessedAt.Equal(*app.ApprovedAt) {
		t.Error("approval time must equal assessment time")
	}

	if app.RejectedAt != nil {
		t.Error("rejectedAt must remain nil")
	}
}

// Kiểm tra hồ sơ được chuyển thẩm định thủ công.
func TestApplyAssessment_ManualReview(t *testing.T) {
	app := newUnderReviewTestApplication(t)
	input := validAssessmentTestInput()

	// DTI = 45%, thuộc vùng thẩm định thủ công.
	input.Affordability.TotalMonthlyDebt = 4_500_000

	result, err := app.ApplyAssessment(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Decision != AssessmentManualReview {
		t.Errorf("expected manual_review, got %s", result.Decision)
	}

	if app.Status != ApplicationStatusUnderReview {
		t.Errorf("expected under_review, got %s", app.Status)
	}

	if app.AssessmentScore != 75 {
		t.Errorf("expected score 75, got %d", app.AssessmentScore)
	}

	if app.AssessedAt == nil {
		t.Error("assessedAt must not be nil")
	}

	if app.ApprovedAt != nil || app.RejectedAt != nil {
		t.Error("manual review must not finalize the application")
	}

	if len(app.AssessmentReasons) == 0 {
		t.Error("manual review must contain reasons")
	}
}

// Kiểm tra hồ sơ bị từ chối tự động.
func TestApplyAssessment_Rejected(t *testing.T) {
	app := newUnderReviewTestApplication(t)
	input := validAssessmentTestInput()

	// DTI lớn hơn 50%.
	input.Affordability.TotalMonthlyDebt = 5_000_001

	result, err := app.ApplyAssessment(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Decision != AssessmentRejected {
		t.Errorf("expected rejected, got %s", result.Decision)
	}

	if app.Status != ApplicationStatusRejected {
		t.Errorf("expected rejected status, got %s", app.Status)
	}

	if app.RejectedAt == nil || app.AssessedAt == nil {
		t.Fatal("rejection and assessment timestamps are required")
	}

	if app.ApprovedAt != nil {
		t.Error("approvedAt must remain nil")
	}

	if len(app.AssessmentReasons) == 0 {
		t.Error("rejected application must have reasons")
	}
}

// Kiểm tra không thể đánh giá hồ sơ sai trạng thái.
func TestApplyAssessment_InvalidStatus(t *testing.T) {
	statuses := []ApplicationStatus{
		ApplicationStatusDraft,
		ApplicationStatusSubmitted,
		ApplicationStatusApproved,
		ApplicationStatusRejected,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			app := newTestApplication(t)
			app.Status = status

			result, err := app.ApplyAssessment(
				validAssessmentTestInput(),
			)

			if !errors.Is(err, ErrInvalidApplicationStatus) {
				t.Errorf(
					"expected ErrInvalidApplicationStatus, got %v",
					err,
				)
			}

			if result != nil {
				t.Error("result must be nil")
			}

			if app.Status != status {
				t.Error("application status must not change")
			}

			if app.AssessedAt != nil {
				t.Error("assessedAt must remain nil")
			}
		})
	}
}

// Kiểm tra không thể xử lý lại hồ sơ đã có quyết định.
func TestApplyAssessment_AlreadyDecided(t *testing.T) {
	tests := []struct {
		name   string
		reject bool
	}{
		{name: "already approved"},
		{name: "already rejected", reject: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newUnderReviewTestApplication(t)
			input := validAssessmentTestInput()

			if tt.reject {
				input.Affordability.TotalMonthlyDebt = 5_000_001
			}

			if _, err := app.ApplyAssessment(input); err != nil {
				t.Fatalf("first assessment failed: %v", err)
			}

			previousStatus := app.Status
			previousUpdatedAt := app.UpdatedAt
			previousScore := app.AssessmentScore

			result, err := app.ApplyAssessment(
				validAssessmentTestInput(),
			)

			if !errors.Is(err, ErrInvalidApplicationStatus) {
				t.Errorf("expected status error, got %v", err)
			}

			if result != nil {
				t.Error("second assessment must return nil")
			}

			if app.Status != previousStatus {
				t.Error("status must not change")
			}

			if !app.UpdatedAt.Equal(previousUpdatedAt) {
				t.Error("updatedAt must not change")
			}

			if app.AssessmentScore != previousScore {
				t.Error("assessment score must not change")
			}
		})
	}
}

// Kiểm tra dữ liệu đánh giá không hợp lệ không làm thay đổi hồ sơ.
func TestApplyAssessment_InvalidInput(t *testing.T) {
	app := newUnderReviewTestApplication(t)
	input := validAssessmentTestInput()

	input.MonthlyIncome = 0

	previousUpdatedAt := app.UpdatedAt

	result, err := app.ApplyAssessment(input)

	if err == nil {
		t.Fatal("expected validation error")
	}

	if result != nil {
		t.Error("result must be nil")
	}

	if app.Status != ApplicationStatusUnderReview {
		t.Error("status must remain under_review")
	}

	if app.AssessedAt != nil {
		t.Error("assessedAt must remain nil")
	}

	if !app.UpdatedAt.Equal(previousUpdatedAt) {
		t.Error("updatedAt must not change")
	}
}

// Kiểm tra trường hợp hồ sơ không tồn tại.
func TestApplyAssessment_NilApplication(t *testing.T) {
	var app *Application

	result, err := app.ApplyAssessment(
		validAssessmentTestInput(),
	)

	if err == nil {
		t.Fatal("expected error for nil application")
	}

	if result != nil {
		t.Error("result must be nil")
	}
}
