package commands

import (
	"context"
	"fmt"
	"time"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
)

const loanDocumentReviewedEvent = "LoanDocumentReviewed"

// ReviewLoanDocumentCommand ghi kết quả duyệt một chứng từ.
// ActorID phải lấy từ danh tính đã xác thực.
type ReviewLoanDocumentCommand struct {
	ApplicationID string
	DocumentID    string
	Decision      string
	Reason        string
	ActorID       string
}

type ReviewLoanDocumentCommandHandler struct {
	Repo       LoanDocumentRepository
	Authorizer LoanReviewAuthorizer
}

func (h *ReviewLoanDocumentCommandHandler) Handle(
	ctx context.Context,
	cmd ReviewLoanDocumentCommand,
) (*loandomain.Application, error) {
	if h == nil || h.Repo == nil || h.Authorizer == nil {
		return nil, fmt.Errorf("loan document review is not configured")
	}
	if err := h.Authorizer.AuthorizeLoanReview(ctx, cmd.ActorID); err != nil {
		return nil, fmt.Errorf("loan review authorization failed: %w", err)
	}
	app, revision, err := h.Repo.FindByIDWithRevision(ctx, cmd.ApplicationID)
	if err != nil {
		return nil, fmt.Errorf("find loan application failed: %w", err)
	}
	if app == nil {
		return nil, fmt.Errorf("loan application not found")
	}

	reviewedAt := time.Now().UTC()
	preview := *app
	if err := preview.ReviewDocument(
		cmd.DocumentID, cmd.Decision, cmd.Reason, cmd.ActorID, reviewedAt,
	); err != nil {
		return nil, err
	}

	payload := struct {
		DocumentId string    `json:"documentId"`
		Decision   string    `json:"decision"`
		Reason     string    `json:"reason"`
		ReviewerId string    `json:"reviewerId"`
		ReviewedAt time.Time `json:"reviewedAt"`
	}{cmd.DocumentID, cmd.Decision, cmd.Reason, cmd.ActorID, reviewedAt}

	if err := h.Repo.Append(
		ctx, app.Id, revision, loanDocumentReviewedEvent, payload,
	); err != nil {
		return nil, fmt.Errorf("append loan document review failed: %w", err)
	}
	return &preview, nil
}
