package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
)

const loanApplicationSubmittedEvent = "LoanApplicationSubmitted"

// SubmitLoanApplicationCommand chứa thông tin
// yêu cầu nộp hồ sơ vay vốn.
type SubmitLoanApplicationCommand struct {
	ApplicationID string
	CustomerID    string
}

// SubmitLoanApplicationCommandHandler xử lý
// nghiệp vụ nộp hồ sơ vay.
type SubmitLoanApplicationCommandHandler struct {
	repo LoanDocumentRepository
}

func NewSubmitLoanApplicationCommandHandler(
	repo LoanDocumentRepository,
) *SubmitLoanApplicationCommandHandler {
	return &SubmitLoanApplicationCommandHandler{
		repo: repo,
	}
}

func (h *SubmitLoanApplicationCommandHandler) Handle(
	ctx context.Context,
	cmd SubmitLoanApplicationCommand,
) (*loandomain.Application, error) {
	// 1. Kiểm tra dependency.
	if h == nil || h.repo == nil {
		return nil, fmt.Errorf(
			"loan repository is not configured",
		)
	}

	// 2. Chuẩn hóa dữ liệu.
	cmd.ApplicationID = strings.TrimSpace(cmd.ApplicationID)
	cmd.CustomerID = strings.TrimSpace(cmd.CustomerID)

	if cmd.ApplicationID == "" || cmd.CustomerID == "" {
		return nil, fmt.Errorf(
			"application ID and customer ID are required",
		)
	}

	// 3. Đọc hồ sơ và revision từ EventStoreDB.
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
		return nil, fmt.Errorf(
			"loan application not found",
		)
	}

	// 4. Kiểm tra quyền sở hữu hồ sơ.
	if app.CustomerId != cmd.CustomerID {
		return nil, fmt.Errorf(
			"customer is not authorized for this loan application",
		)
	}

	// 5. Tạo bản sao để xử lý nghiệp vụ.
	// Không thay đổi trực tiếp aggregate đã đọc.
	preview := *app

	// 6. Domain kiểm tra:
	// - Trạng thái phải là draft.
	// - Phải đủ 7 loại chứng từ.
	// - Chứng từ rejected không được tính.
	if err := preview.Submit(); err != nil {
		return nil, fmt.Errorf(
			"cannot submit loan application: %w",
			err,
		)
	}

	if preview.SubmittedAt == nil {
		return nil, fmt.Errorf(
			"loan submission timestamp is missing",
		)
	}

	// 7. Chuẩn bị Event Payload.
	// Sử dụng thời gian do Domain tạo ra.
	payload := struct {
		SubmittedAt time.Time `json:"submittedAt"`
		UpdatedAt   time.Time `json:"updatedAt"`
	}{
		SubmittedAt: *preview.SubmittedAt,
		UpdatedAt:   preview.UpdatedAt,
	}

	// 8. Ghi sự kiện vào EventStoreDB.
	if err := h.repo.Append(
		ctx,
		app.Id,
		revision,
		loanApplicationSubmittedEvent,
		payload,
	); err != nil {
		return nil, fmt.Errorf(
			"append loan submission event failed: %w",
			err,
		)
	}

	// 9. Chỉ trả kết quả thành công sau khi lưu event.
	return &preview, nil
}
