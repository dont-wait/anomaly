package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
)

const loanReviewRejectedEvent = "LoanApplicationReviewRejected"

// RejectLoanReviewCommandHandler từ chối hồ sơ đang thẩm định khi chưa chấm điểm.
// ActorID phải lấy từ danh tính đã xác thực.
type RejectLoanReviewCommandHandler struct {
	Repo       LoanDocumentRepository
	Authorizer LoanManualDecisionAuthorizer
}

func (h *RejectLoanReviewCommandHandler) Handle(
	ctx context.Context,
	applicationID, actorID, reason string,
) (*loandomain.Application, error) {
	if h == nil || h.Repo == nil || h.Authorizer == nil {
		return nil, fmt.Errorf("loan review rejection is not configured")
	}
	actorID = strings.TrimSpace(actorID)
	if err := h.Authorizer.AuthorizeLoanManualDecision(ctx, actorID); err != nil {
		return nil, fmt.Errorf("loan review rejection authorization failed: %w", err)
	}
	app, revision, err := h.Repo.FindByIDWithRevision(ctx, applicationID)
	if err != nil {
		return nil, fmt.Errorf("find loan application failed: %w", err)
	}
	if app == nil {
		return nil, fmt.Errorf("loan application not found")
	}

	at := time.Now().UTC()
	preview := *app
	if err := preview.RejectDuringReview(actorID, reason, at); err != nil {
		return nil, err
	}

	// Tên JSON khớp loanDecisionPayload ở infrastructure/eventstore.
	payload := struct {
		DecidedAt            time.Time `json:"decidedAt"`
		UpdatedAt            time.Time `json:"updatedAt"`
		ManualReviewerId     string    `json:"manualReviewerId"`
		ManualDecisionReason string    `json:"manualDecisionReason"`
	}{at, at, preview.ManualReviewerId, preview.ManualDecisionReason}

	if err := h.Repo.Append(ctx, app.Id, revision, loanReviewRejectedEvent, payload); err != nil {
		return nil, fmt.Errorf("append loan review rejection failed: %w", err)
	}
	return &preview, nil
}
