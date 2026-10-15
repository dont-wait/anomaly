package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
)

const loanDocumentAddedEvent = "LoanDocumentAdded"

// AddLoanDocumentCommand chứa thông tin chứng từ
// cần liên kết với hồ sơ vay.
type AddLoanDocumentCommand struct {
	ApplicationID string
	CustomerID    string
	Document      loandomain.ApplicationDocument
}

// AddLoanDocumentCommandHandler điều phối nghiệp vụ
// thêm chứng từ vào hồ sơ vay.
type AddLoanDocumentCommandHandler struct {
	repo     LoanDocumentRepository
	verifier LoanDocumentStorageVerifier
}

func NewAddLoanDocumentCommandHandler(
	repo LoanDocumentRepository,
	verifier LoanDocumentStorageVerifier,
) *AddLoanDocumentCommandHandler {
	return &AddLoanDocumentCommandHandler{
		repo:     repo,
		verifier: verifier,
	}
}

func (h *AddLoanDocumentCommandHandler) Handle(
	ctx context.Context,
	cmd AddLoanDocumentCommand,
) (*loandomain.Application, error) {
	// 1. Kiểm tra dependency.
	if h == nil || h.repo == nil {
		return nil, fmt.Errorf(
			"loan document repository is not configured",
		)
	}

	if h.verifier == nil {
		return nil, fmt.Errorf(
			"loan document storage verifier is not configured",
		)
	}

	// 2. Chuẩn hóa định danh.
	cmd.ApplicationID = strings.TrimSpace(cmd.ApplicationID)
	cmd.CustomerID = strings.TrimSpace(cmd.CustomerID)

	if cmd.ApplicationID == "" || cmd.CustomerID == "" {
		return nil, fmt.Errorf(
			"application ID and customer ID are required",
		)
	}

	// 3. Đọc hồ sơ từ EventStoreDB và lấy revision.
	app, revision, err := h.repo.FindByIDWithRevision(
		ctx,
		cmd.ApplicationID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"find loan application failed: %w",
			err,
		)
	}

	if app == nil {
		return nil, fmt.Errorf("loan application not found")
	}

	// 4. Kiểm tra hồ sơ thuộc khách hàng.
	//
	// CustomerID phải được HTTP Layer lấy từ
	// JWT đã xác thực, không lấy trực tiếp từ body.
	if app.CustomerId != cmd.CustomerID {
		return nil, fmt.Errorf(
			"customer is not authorized for this loan application",
		)
	}

	// 5. Tạo bản sao hồ sơ để kiểm tra bằng Domain.
	// Không chỉnh sửa trực tiếp đối tượng đã đọc.
	preview := *app
	preview.Documents = append(
		[]loandomain.ApplicationDocument(nil),
		app.Documents...,
	)

	// 6. Domain kiểm tra metadata, trạng thái draft
	// và DocumentId trùng lặp.
	if err := preview.AddDocument(cmd.Document); err != nil {
		return nil, fmt.Errorf(
			"invalid loan document: %w",
			err,
		)
	}

	// Document sau khi Domain xử lý sẽ có
	// VerificationStatus = pending và UploadedAt.
	savedDocument := preview.Documents[len(preview.Documents)-1]
	// 7. Xác minh đối tượng lưu trữ thực tế.
	//
	// Verifier phải kiểm tra quyền sở hữu StorageKey,
	// sự tồn tại của file, SizeBytes, SHA256 và MIMEType.
	if err := h.verifier.VerifyLoanDocument(
		ctx,
		app.CustomerId,
		app.Id,
		savedDocument,
	); err != nil {
		return nil, fmt.Errorf(
			"loan document storage verification failed: %w",
			err,
		)
	}

	// 8. Chuẩn bị payload cho LoanDocumentAdded.
	payload := struct {
		Document  loandomain.ApplicationDocument `json:"document"`
		UpdatedAt time.Time                      `json:"updatedAt"`
	}{
		Document:  savedDocument,
		UpdatedAt: preview.UpdatedAt,
	}

	// 9. Ghi event với optimistic concurrency.
	if err := h.repo.Append(
		ctx,
		app.Id,
		revision,
		loanDocumentAddedEvent,
		payload,
	); err != nil {
		return nil, fmt.Errorf(
			"append loan document event failed: %w",
			err,
		)
	}

	// 10. Chỉ trả kết quả sau khi ghi thành công.
	return &preview, nil
}
