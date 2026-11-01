package commands

import (
	"context"
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/google/uuid"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
)

// LoanProfileDocumentProvider trả chứng từ đã xác minh trong hồ sơ cá nhân
// của khách. Nguồn là dữ liệu máy chủ, không nhận từ client.
type LoanProfileDocumentProvider interface {
	VerifiedProfileDocuments(
		ctx context.Context,
		customerID string,
	) ([]loandomain.ApplicationDocument, error)
}

// LoanDocumentSnapshotStorage sao chép file sang vùng lưu của hồ sơ vay và đối
// chiếu lại bản sao trước khi ghi nhận.
type LoanDocumentSnapshotStorage interface {
	LoanDocumentStorageVerifier
	CopyLoanDocument(ctx context.Context, sourceKey, targetKey, mimeType string) error
}

// AttachProfileDocumentsCommandHandler chụp chứng từ trong hồ sơ cá nhân vào hồ
// sơ vay. Mỗi hồ sơ giữ bản sao riêng, nên khách đổi hoặc xóa chứng từ ở profile
// không ảnh hưởng hồ sơ đã nộp. Chứng từ vẫn chờ duyệt như mọi chứng từ khác, và
// gọi lại nhiều lần không tạo bản trùng.
type AttachProfileDocumentsCommandHandler struct {
	Repo     LoanDocumentRepository
	Provider LoanProfileDocumentProvider
	Storage  LoanDocumentSnapshotStorage
}

var profileDocumentNamespace = uuid.MustParse("6f1d7c52-3b5e-4a58-9c43-0d6e2a7b91f4")

func (h *AttachProfileDocumentsCommandHandler) Handle(
	ctx context.Context,
	applicationID, customerID string,
) (*loandomain.Application, error) {
	if h == nil || h.Repo == nil || h.Provider == nil || h.Storage == nil {
		return nil, fmt.Errorf("loan profile documents are not configured")
	}
	app, revision, err := h.Repo.FindByIDWithRevision(ctx, applicationID)
	if err != nil {
		return nil, fmt.Errorf("find loan application failed: %w", err)
	}
	if app == nil {
		return nil, fmt.Errorf("loan application not found")
	}
	if app.CustomerId != customerID {
		return nil, fmt.Errorf("customer is not authorized for this loan application")
	}
	if app.Status != loandomain.ApplicationStatusDraft {
		return app, nil
	}
	documents, err := h.Provider.VerifiedProfileDocuments(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("read profile documents failed: %w", err)
	}

	result := *app
	result.Documents = append([]loandomain.ApplicationDocument(nil), app.Documents...)
	var failures []error
	for _, document := range documents {
		// Có bản bất kỳ, kể cả bị từ chối, thì không tự thay thế.
		if hasDocumentType(&result, document.Type) {
			continue
		}
		sourceKey := document.StorageKey
		document.DocumentId = uuid.NewSHA1(
			profileDocumentNamespace,
			[]byte(app.Id+":"+document.Type),
		).String()
		document.StorageKey = "loans/" + customerID + "/" + app.Id + "/" +
			document.DocumentId + path.Ext(sourceKey)

		preview := result
		preview.Documents = append([]loandomain.ApplicationDocument(nil), result.Documents...)
		// Bản ghi cũ thiếu metadata thì bỏ qua để khách tự tải lên.
		if err := preview.AddDocument(document); err != nil || sourceKey == "" {
			continue
		}
		saved := preview.Documents[len(preview.Documents)-1]
		// Một file lỗi không được chặn các chứng từ còn lại.
		if err := h.Storage.CopyLoanDocument(ctx, sourceKey, saved.StorageKey, saved.MIMEType); err != nil {
			failures = append(failures, fmt.Errorf("copy %s: %w", saved.Type, err))
			continue
		}
		if err := h.Storage.VerifyLoanDocument(ctx, customerID, app.Id, saved); err != nil {
			failures = append(failures, fmt.Errorf("verify %s: %w", saved.Type, err))
			continue
		}
		payload := struct {
			Document  loandomain.ApplicationDocument `json:"document"`
			UpdatedAt time.Time                      `json:"updatedAt"`
		}{saved, preview.UpdatedAt}
		if err := h.Repo.Append(ctx, app.Id, revision, loanDocumentAddedEvent, payload); err != nil {
			return nil, fmt.Errorf("append loan document event failed: %w", err)
		}
		revision++
		result = preview
	}
	if len(failures) > 0 && len(result.Documents) == len(app.Documents) {
		return nil, fmt.Errorf("snapshot profile documents failed: %w", errors.Join(failures...))
	}
	return &result, nil
}

func hasDocumentType(app *loandomain.Application, documentType string) bool {
	for _, document := range app.Documents {
		if document.Type == documentType {
			return true
		}
	}
	return false
}
