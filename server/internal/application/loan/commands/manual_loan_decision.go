package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
)

// ManualLoanDecisionCommand chứa quyết định
// phê duyệt hoặc từ chối hồ sơ vay thủ công.
type ManualLoanDecisionCommand struct {
	ApplicationID string
	ActorID       string
	Decision      loandomain.ApplicationStatus
	Reason        string
}

// ManualLoanDecisionCommandHandler xử lý quyết định
// của nhân viên sau khi hệ thống yêu cầu manual_review.
type ManualLoanDecisionCommandHandler struct {
	repo       LoanDocumentRepository
	authorizer LoanManualDecisionAuthorizer
}

func NewManualLoanDecisionCommandHandler(
	repo LoanDocumentRepository,
	authorizer LoanManualDecisionAuthorizer,
) *ManualLoanDecisionCommandHandler {
	return &ManualLoanDecisionCommandHandler{
		repo:       repo,
		authorizer: authorizer,
	}
}

func (h *ManualLoanDecisionCommandHandler) Handle(
	ctx context.Context,
	cmd ManualLoanDecisionCommand,
) (*loandomain.Application, error) {
	// 1. Kiểm tra dependency.
	if h == nil || h.repo == nil || h.authorizer == nil {
		return nil, fmt.Errorf(
			"manual loan decision dependencies are not configured",
		)
	}

	// 2. Chuẩn hóa dữ liệu.
	cmd.ApplicationID = strings.TrimSpace(cmd.ApplicationID)
	cmd.ActorID = strings.TrimSpace(cmd.ActorID)
	cmd.Reason = strings.TrimSpace(cmd.Reason)

	if cmd.ApplicationID == "" || cmd.ActorID == "" {
		return nil, fmt.Errorf(
			"application ID and actor ID are required",
		)
	}

	// Chỉ cho phép hai quyết định cuối cùng.
	if cmd.Decision != loandomain.ApplicationStatusApproved &&
		cmd.Decision != loandomain.ApplicationStatusRejected {
		return nil, fmt.Errorf(
			"manual decision must be approved or rejected",
		)
	}

	if cmd.Reason == "" {
		return nil, fmt.Errorf(
			"manual decision reason is required",
		)
	}

	// 3. Kiểm tra quyền của nhân viên.
	//
	// ActorID phải lấy từ JWT hoặc authentication
	// context đáng tin cậy, không lấy từ request body.
	if err := h.authorizer.AuthorizeLoanManualDecision(
		ctx,
		cmd.ActorID,
	); err != nil {
		return nil, fmt.Errorf(
			"manual loan decision authorization failed: %w",
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

	// 5. Chỉ cho quyết định thủ công khi:
	// - Hồ sơ đang under_review.
	// - Đã chấm điểm.
	// - Kết quả là manual_review.
	if app.Status != loandomain.ApplicationStatusUnderReview ||
		app.AssessedAt == nil ||
		app.AssessmentDecision != loandomain.AssessmentManualReview {
		return nil, fmt.Errorf(
			"loan application is not awaiting a manual decision",
		)
	}

	// 6. Tạo bản sao aggregate để xử lý bằng Domain.
	preview := *app

	if err := preview.ApplyManualDecision(
		cmd.ActorID,
		cmd.Decision,
		cmd.Reason,
	); err != nil {
		return nil, fmt.Errorf(
			"apply manual loan decision failed: %w",
			err,
		)
	}

	// 7. Xác định loại event và thời gian quyết định.
	var eventType string
	var decidedAt time.Time

	switch preview.Status {
	case loandomain.ApplicationStatusApproved:
		eventType = loanApplicationApprovedEvent

		if preview.ApprovedAt == nil {
			return nil, fmt.Errorf(
				"manual approval timestamp is missing",
			)
		}

		decidedAt = *preview.ApprovedAt

	case loandomain.ApplicationStatusRejected:
		eventType = loanApplicationRejectedEvent

		if preview.RejectedAt == nil {
			return nil, fmt.Errorf(
				"manual rejection timestamp is missing",
			)
		}

		decidedAt = *preview.RejectedAt

	default:
		return nil, fmt.Errorf(
			"unexpected manual decision status: %s",
			preview.Status,
		)
	}

	if preview.ManualDecidedAt == nil {
		return nil, fmt.Errorf(
			"manual decision timestamp is missing",
		)
	}

	if !preview.ManualDecidedAt.Equal(decidedAt) {
		return nil, fmt.Errorf(
			"manual decision timestamps do not match",
		)
	}

	// 8. Chuẩn bị payload ghi vào EventStoreDB.
	// Tên JSON phải khớp loanDecisionPayload
	// ở infrastructure/eventstore/loan_events.go.
	payload := struct {
		DecidedAt            time.Time  `json:"decidedAt"`
		UpdatedAt            time.Time  `json:"updatedAt"`
		ManualReviewerId     string     `json:"manualReviewerId"`
		ManualDecisionReason string     `json:"manualDecisionReason"`
		ManualDecidedAt      *time.Time `json:"manualDecidedAt"`
	}{
		DecidedAt:            decidedAt,
		UpdatedAt:            preview.UpdatedAt,
		ManualReviewerId:     preview.ManualReviewerId,
		ManualDecisionReason: preview.ManualDecisionReason,
		ManualDecidedAt:      preview.ManualDecidedAt,
	}

	// 9. Ghi event với optimistic concurrency.
	if err := h.repo.Append(
		ctx,
		app.Id,
		revision,
		eventType,
		payload,
	); err != nil {
		return nil, fmt.Errorf(
			"append manual loan decision event failed: %w",
			err,
		)
	}

	// 10. Chỉ trả thành công sau khi ghi event.
	return &preview, nil
}
