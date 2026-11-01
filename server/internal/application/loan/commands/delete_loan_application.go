package commands

import (
	"context"
	"fmt"
	"time"
)

const loanApplicationDeletedEvent = "LoanApplicationDeleted"

// DeleteLoanApplicationCommandHandler cho khách xóa hồ sơ chưa gửi.
// CustomerID phải lấy từ danh tính đã xác thực.
type DeleteLoanApplicationCommandHandler struct {
	Repo LoanDocumentRepository
}

func (h *DeleteLoanApplicationCommandHandler) Handle(
	ctx context.Context,
	applicationID, customerID string,
) error {
	if h == nil || h.Repo == nil {
		return fmt.Errorf("loan repository is not configured")
	}
	app, revision, err := h.Repo.FindByIDWithRevision(ctx, applicationID)
	if err != nil {
		return fmt.Errorf("find loan application failed: %w", err)
	}
	if app == nil {
		return fmt.Errorf("loan application not found")
	}
	if app.CustomerId != customerID {
		return fmt.Errorf("customer is not authorized for this loan application")
	}
	deletedAt := time.Now().UTC()
	preview := *app
	if err := preview.Delete(deletedAt); err != nil {
		return err
	}
	payload := struct {
		DeletedAt time.Time `json:"deletedAt"`
	}{deletedAt}
	if err := h.Repo.Append(ctx, app.Id, revision, loanApplicationDeletedEvent, payload); err != nil {
		return fmt.Errorf("append loan delete event failed: %w", err)
	}
	return nil
}
