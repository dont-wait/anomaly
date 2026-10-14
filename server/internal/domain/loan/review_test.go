package loan

import (
	"errors"
	"testing"
	"time"
)

// Tạo hồ sơ đã submitted để phục vụ kiểm thử.
func newSubmittedTestApplication(t *testing.T) *Application {
	t.Helper()

	app := newTestApplication(t)

	addTestLoanDocuments(
		t,
		app,
		testRequiredLoanDocuments(),
	)

	if err := app.Submit(); err != nil {
		t.Fatalf("failed to submit application: %v", err)
	}

	return app
}

// Kiểm tra bắt đầu thẩm định thành công.
func TestStartReview_Success(t *testing.T) {
	app := newSubmittedTestApplication(t)

	submittedAt := *app.SubmittedAt

	before := time.Now().UTC()
	err := app.StartReview()
	after := time.Now().UTC()

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if app.Status != ApplicationStatusUnderReview {
		t.Errorf(
			"expected under_review, got %s",
			app.Status,
		)
	}

	if app.ReviewStartedAt == nil {
		t.Fatal("reviewStartedAt must not be nil")
	}

	if app.ReviewStartedAt.Before(before) ||
		app.ReviewStartedAt.After(after) {
		t.Error("reviewStartedAt is outside expected time range")
	}

	if !app.UpdatedAt.Equal(*app.ReviewStartedAt) {
		t.Error("updatedAt must equal reviewStartedAt")
	}

	if !app.SubmittedAt.Equal(submittedAt) {
		t.Error("submittedAt must not change")
	}
}

// Kiểm tra các trạng thái không được bắt đầu thẩm định.
func TestStartReview_InvalidStatus(t *testing.T) {
	tests := []struct {
		name   string
		status ApplicationStatus
	}{
		{
			name:   "draft",
			status: ApplicationStatusDraft,
		},
		{
			name:   "under review",
			status: ApplicationStatusUnderReview,
		},
		{
			name:   "approved",
			status: ApplicationStatusApproved,
		},
		{
			name:   "rejected",
			status: ApplicationStatusRejected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newTestApplication(t)
			app.Status = tt.status

			previousUpdatedAt := app.UpdatedAt

			err := app.StartReview()

			if !errors.Is(err, ErrInvalidApplicationStatus) {
				t.Errorf(
					"expected ErrInvalidApplicationStatus, got %v",
					err,
				)
			}

			if app.Status != tt.status {
				t.Error("application status must not change")
			}

			if app.ReviewStartedAt != nil {
				t.Error("reviewStartedAt must remain nil")
			}

			if !app.UpdatedAt.Equal(previousUpdatedAt) {
				t.Error("updatedAt must not change")
			}
		})
	}
}

// Kiểm tra không thể bắt đầu thẩm định hai lần.
func TestStartReview_Twice(t *testing.T) {
	app := newSubmittedTestApplication(t)

	if err := app.StartReview(); err != nil {
		t.Fatalf("first review failed: %v", err)
	}

	firstReviewStartedAt := *app.ReviewStartedAt
	firstUpdatedAt := app.UpdatedAt

	err := app.StartReview()

	if !errors.Is(err, ErrInvalidApplicationStatus) {
		t.Errorf(
			"expected ErrInvalidApplicationStatus, got %v",
			err,
		)
	}

	if !app.ReviewStartedAt.Equal(firstReviewStartedAt) {
		t.Error("reviewStartedAt must not change")
	}

	if !app.UpdatedAt.Equal(firstUpdatedAt) {
		t.Error("updatedAt must not change")
	}

	if app.Status != ApplicationStatusUnderReview {
		t.Error("status must remain under_review")
	}
}

// Kiểm tra hồ sơ nil.
func TestStartReview_NilApplication(t *testing.T) {
	var app *Application

	err := app.StartReview()

	if err == nil {
		t.Fatal("expected error for nil application")
	}
}
