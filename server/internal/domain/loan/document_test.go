package loan

import (
	"errors"
	"testing"
)

// Tạo hồ sơ vay mẫu phục vụ kiểm thử.
func newTestApplication(t *testing.T) *Application {
	t.Helper()

	app, err := NewDraftApplication(
		"app_001",
		"LA20261014001",
		"customer_001",
		"package_001",
		"account_001",
		50000000,
		12,
	)
	if err != nil {
		t.Fatalf("failed to create application: %v", err)
	}

	return app
}

// Tạo metadata chứng từ hợp lệ.
func validTestDocument() ApplicationDocument {
	return ApplicationDocument{
		DocumentId: "doc_001",
		Type:       "CCCD_FRONT",
		StorageKey: "loans/app_001/doc_001.jpg",
		MIMEType:   "image/jpeg",
		SizeBytes:  102400,
		SHA256:     "test_sha256_hash",
	}
}

// Kiểm tra thêm chứng từ thành công.
func TestAddDocument_Success(t *testing.T) {
	app := newTestApplication(t)
	doc := validTestDocument()

	err := app.AddDocument(doc)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(app.Documents) != 1 {
		t.Fatalf("expected 1 document, got %d", len(app.Documents))
	}

	saved := app.Documents[0]

	if saved.DocumentId != doc.DocumentId {
		t.Errorf("unexpected document ID: %s", saved.DocumentId)
	}

	if saved.StorageKey != doc.StorageKey {
		t.Errorf("unexpected storage key: %s", saved.StorageKey)
	}

	if saved.VerificationStatus != "pending" {
		t.Errorf("expected pending, got %s", saved.VerificationStatus)
	}

	if saved.UploadedAt.IsZero() {
		t.Error("uploadedAt must not be zero")
	}

	if !app.UpdatedAt.Equal(saved.UploadedAt) {
		t.Error("updatedAt must equal uploadedAt")
	}
}

// Kiểm tra không cho thêm chứng từ bị trùng.
func TestAddDocument_Duplicate(t *testing.T) {
	app := newTestApplication(t)
	doc := validTestDocument()

	if err := app.AddDocument(doc); err != nil {
		t.Fatalf("first upload failed: %v", err)
	}

	err := app.AddDocument(doc)

	if err == nil {
		t.Fatal("expected duplicate document error")
	}

	if len(app.Documents) != 1 {
		t.Errorf("expected 1 document, got %d", len(app.Documents))
	}
}

// Kiểm tra trạng thái hồ sơ không hợp lệ.
func TestAddDocument_InvalidStatus(t *testing.T) {
	app := newTestApplication(t)
	app.Status = ApplicationStatusSubmitted

	err := app.AddDocument(validTestDocument())

	if !errors.Is(err, ErrInvalidApplicationStatus) {
		t.Errorf("expected ErrInvalidApplicationStatus, got %v", err)
	}

	if len(app.Documents) != 0 {
		t.Error("document must not be added")
	}
}

// Kiểm tra metadata chứng từ không hợp lệ.
func TestAddDocument_InvalidMetadata(t *testing.T) {
	tests := []struct {
		name   string
		change func(*ApplicationDocument)
	}{
		{
			name: "empty document ID",
			change: func(d *ApplicationDocument) {
				d.DocumentId = ""
			},
		},
		{
			name: "empty document type",
			change: func(d *ApplicationDocument) {
				d.Type = ""
			},
		},
		{
			name: "empty storage key",
			change: func(d *ApplicationDocument) {
				d.StorageKey = ""
			},
		},
		{
			name: "empty MIME type",
			change: func(d *ApplicationDocument) {
				d.MIMEType = ""
			},
		},
		{
			name: "empty SHA256",
			change: func(d *ApplicationDocument) {
				d.SHA256 = ""
			},
		},
		{
			name: "invalid file size",
			change: func(d *ApplicationDocument) {
				d.SizeBytes = 0
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newTestApplication(t)
			doc := validTestDocument()

			tt.change(&doc)

			err := app.AddDocument(doc)

			if err == nil {
				t.Fatal("expected validation error")
			}

			if len(app.Documents) != 0 {
				t.Error("invalid document must not be added")
			}
		})
	}
}

// Kiểm tra khi hồ sơ vay là nil.
func TestAddDocument_NilApplication(t *testing.T) {
	var app *Application

	err := app.AddDocument(validTestDocument())

	if err == nil {
		t.Fatal("expected error for nil application")
	}
}
