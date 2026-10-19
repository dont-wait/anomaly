package loan

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

// Danh sách chứng từ bắt buộc trong hồ sơ vay.
func testRequiredLoanDocuments() []string {
	return []string{
		"IDENTITY_FRONT",
		"IDENTITY_BACK",
		"VNEID_PERSONAL",
		"VNEID_RESIDENCE",
		"VSSID_PARTICIPATION",
		"VSSID_DETAIL",
		"SALARY_STATEMENT_3_MONTHS",
	}
}

// Thêm chứng từ mẫu vào hồ sơ phục vụ kiểm thử.
func addTestLoanDocuments(
	t *testing.T,
	app *Application,
	types []string,
) {
	t.Helper()

	for i, docType := range types {
		doc := validTestDocument()

		doc.DocumentId = fmt.Sprintf("doc_%03d", i+1)
		doc.Type = docType
		doc.StorageKey = fmt.Sprintf(
			"loans/app_001/doc_%03d.jpg", i+1,
		)

		if err := app.AddDocument(doc); err != nil {
			t.Fatalf("failed to add document %s: %v",
				docType, err)
		}
	}
}

// Kiểm tra gửi hồ sơ thành công khi đủ chứng từ.
func TestSubmit_Success(t *testing.T) {
	app := newTestApplication(t)

	addTestLoanDocuments(
		t, app, testRequiredLoanDocuments(),
	)

	before := time.Now().UTC()
	err := app.Submit()
	after := time.Now().UTC()

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if app.Status != ApplicationStatusSubmitted {
		t.Errorf("expected submitted status, got %s",
			app.Status)
	}

	if app.SubmittedAt == nil {
		t.Fatal("submittedAt must not be nil")
	}

	if app.SubmittedAt.Before(before) ||
		app.SubmittedAt.After(after) {
		t.Error("submittedAt is outside expected time range")
	}

	if !app.UpdatedAt.Equal(*app.SubmittedAt) {
		t.Error("updatedAt must equal submittedAt")
	}

	if len(app.Documents) != 7 {
		t.Errorf("expected 7 documents, got %d",
			len(app.Documents))
	}
}

// Kiểm tra gửi hồ sơ khi thiếu chứng từ.
func TestSubmit_MissingDocuments(t *testing.T) {
	app := newTestApplication(t)

	required := testRequiredLoanDocuments()

	// Chỉ thêm 5/7 loại chứng từ.
	addTestLoanDocuments(t, app, required[:5])

	err := app.Submit()

	if err == nil {
		t.Fatal("expected missing documents error")
	}

	var missingErr *MissingLoanDocumentsError

	if !errors.As(err, &missingErr) {
		t.Fatalf("expected MissingLoanDocumentsError, got %v", err)
	}

	expected := []string{
		"VSSID_DETAIL",
		"SALARY_STATEMENT_3_MONTHS",
	}

	if !reflect.DeepEqual(missingErr.Types, expected) {
		t.Errorf("expected missing %v, got %v",
			expected, missingErr.Types)
	}

	if app.Status != ApplicationStatusDraft {
		t.Error("status must remain draft")
	}

	if app.SubmittedAt != nil {
		t.Error("submittedAt must remain nil")
	}
}

// Kiểm tra chứng từ bị từ chối không được tính.
func TestSubmit_RejectedDocument(t *testing.T) {
	app := newTestApplication(t)

	addTestLoanDocuments(
		t, app, testRequiredLoanDocuments(),
	)

	// Giả lập một chứng từ bị từ chối.
	for i := range app.Documents {
		if app.Documents[i].Type == "VSSID_DETAIL" {
			app.Documents[i].VerificationStatus = "rejected"
		}
	}

	err := app.Submit()

	var missingErr *MissingLoanDocumentsError

	if !errors.As(err, &missingErr) {
		t.Fatalf("expected missing documents error, got %v", err)
	}

	expected := []string{"VSSID_DETAIL"}

	if !reflect.DeepEqual(missingErr.Types, expected) {
		t.Errorf("expected %v, got %v",
			expected, missingErr.Types)
	}

	if app.Status != ApplicationStatusDraft {
		t.Error("status must remain draft")
	}
}

// Kiểm tra không thể gửi hồ sơ khi sai trạng thái.
func TestSubmit_InvalidStatus(t *testing.T) {
	app := newTestApplication(t)

	addTestLoanDocuments(
		t, app, testRequiredLoanDocuments(),
	)

	app.Status = ApplicationStatusUnderReview

	err := app.Submit()

	if !errors.Is(err, ErrInvalidApplicationStatus) {
		t.Errorf("expected ErrInvalidApplicationStatus, got %v",
			err)
	}

	if app.SubmittedAt != nil {
		t.Error("submittedAt must remain nil")
	}
}

// Kiểm tra không cho gửi hồ sơ hai lần.
func TestSubmit_Twice(t *testing.T) {
	app := newTestApplication(t)

	addTestLoanDocuments(
		t, app, testRequiredLoanDocuments(),
	)

	if err := app.Submit(); err != nil {
		t.Fatalf("first submit failed: %v", err)
	}

	firstSubmittedAt := *app.SubmittedAt

	err := app.Submit()

	if !errors.Is(err, ErrInvalidApplicationStatus) {
		t.Errorf("expected ErrInvalidApplicationStatus, got %v",
			err)
	}

	if !app.SubmittedAt.Equal(firstSubmittedAt) {
		t.Error("submittedAt must not change")
	}
}

// Kiểm tra khi hồ sơ không tồn tại.
func TestSubmit_NilApplication(t *testing.T) {
	var app *Application

	err := app.Submit()

	if err == nil {
		t.Fatal("expected error for nil application")
	}
}
