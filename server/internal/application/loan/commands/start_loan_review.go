package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
)

const loanReviewStartedEvent = "LoanReviewStarted"

// StartLoanReviewCommand chứa thông tin yêu cầu
// bắt đầu thẩm định hồ sơ vay.
type StartLoanReviewCommand struct {
	ApplicationID string
	ActorID       string
}

// StartLoanReviewCommandHandler xử lý việc chuyển
// hồ sơ từ submitted sang under_review.
type StartLoanReviewCommandHandler struct {
	repo       LoanDocumentRepository
	authorizer LoanReviewAuthorizer
}

func NewStartLoanReviewCommandHandler(
	repo LoanDocumentRepository,
	authorizer LoanReviewAuthorizer,
) *StartLoanReviewCommandHandler {
	return &StartLoanReviewCommandHandler{
		repo:       repo,
		authorizer: authorizer,
	}
}

func (h *StartLoanReviewCommandHandler) Handle(
	ctx context.Context,
	cmd StartLoanReviewCommand,
) (*loandomain.Application, error) {
	// 1. Kiểm tra dependency.
	if h == nil || h.repo == nil {
		return nil, fmt.Errorf(
			"loan repository is not configured",
		)
	}

	if h.authorizer == nil {
		return nil, fmt.Errorf(
			"loan review authorizer is not configured",
		)
	}

	// 2. Chuẩn hóa dữ liệu.
	cmd.ApplicationID = strings.TrimSpace(cmd.ApplicationID)
	cmd.ActorID = strings.TrimSpace(cmd.ActorID)

	if cmd.ApplicationID == "" || cmd.ActorID == "" {
		return nil, fmt.Errorf(
			"application ID and actor ID are required",
		)
	}

	// 3. Kiểm tra quyền thẩm định.
	//
	// ActorID phải lấy từ danh tính đã xác thực.
	// Không được tin giá trị tự khai báo từ request body.
	if err := h.authorizer.AuthorizeLoanReview(
		ctx,
		cmd.ActorID,
	); err != nil {
		return nil, fmt.Errorf(
			"loan review authorization failed: %w",
			err,
		)
	}

	// 4. Đọc hồ sơ và revision từ EventStoreDB.
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

	if app.Id != cmd.ApplicationID {
		return nil, fmt.Errorf(
			"loan application ID mismatch",
		)
	}

	// 5. Tạo bản sao hồ sơ.
	preview := *app

	// 6. Domain kiểm tra trạng thái submitted
	// và chuyển sang under_review.
	if err := preview.StartReview(); err != nil {
		return nil, fmt.Errorf(
			"cannot start loan review: %w",
			err,
		)
	}
	preview.ReviewStartedBy = cmd.ActorID
	if preview.ReviewStartedAt == nil {
		return nil, fmt.Errorf(
			"review start timestamp is missing",
		)
	}

	// 7. Chuẩn bị Event Payload.
	payload := struct {
		ReviewStartedAt time.Time `json:"reviewStartedAt"`
		ReviewerId      string    `json:"reviewerId"`
		UpdatedAt       time.Time `json:"updatedAt"`
	}{
		ReviewStartedAt: *preview.ReviewStartedAt,
		ReviewerId:      cmd.ActorID,
		UpdatedAt:       preview.UpdatedAt,
	}

	// 8. Ghi sự kiện vào EventStoreDB.
	if err := h.repo.Append(
		ctx,
		app.Id,
		revision,
		loanReviewStartedEvent,
		payload,
	); err != nil {
		return nil, fmt.Errorf(
			"append loan review event failed: %w",
			err,
		)
	}

	// 9. Chỉ trả kết quả sau khi ghi thành công.
	return &preview, nil
}
