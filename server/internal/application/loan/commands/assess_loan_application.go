package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
)

const (
	loanApplicationAssessedEvent = "LoanApplicationAssessed"
	loanApplicationApprovedEvent = "LoanApplicationApproved"
	loanApplicationRejectedEvent = "LoanApplicationRejected"

	// Phiên bản chính sách chấm điểm hiện tại.
	loanAssessmentPolicyVersion = "loan-policy-v1"
)

// AssessLoanApplicationCommand yêu cầu thẩm định
// một hồ sơ vay đã được nộp.
type AssessLoanApplicationCommand struct {
	ApplicationID string
	ActorID       string
}

// Handler điều phối quá trình thẩm định.
type AssessLoanApplicationCommandHandler struct {
	repo       LoanAssessmentRepository
	authorizer LoanAssessmentAuthorizer
	data       LoanAssessmentDataProvider
}

func NewAssessLoanApplicationCommandHandler(
	repo LoanAssessmentRepository,
	authorizer LoanAssessmentAuthorizer,
	data LoanAssessmentDataProvider,
) *AssessLoanApplicationCommandHandler {
	return &AssessLoanApplicationCommandHandler{
		repo:       repo,
		authorizer: authorizer,
		data:       data,
	}
}

func (h *AssessLoanApplicationCommandHandler) Handle(
	ctx context.Context,
	cmd AssessLoanApplicationCommand,
) (*loandomain.Application, error) {
	// 1. Kiểm tra dependency.
	if h == nil ||
		h.repo == nil ||
		h.authorizer == nil ||
		h.data == nil {
		return nil, fmt.Errorf(
			"loan assessment dependencies are not configured",
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
	if err := h.authorizer.AuthorizeLoanAssessment(
		ctx,
		cmd.ActorID,
	); err != nil {
		return nil, fmt.Errorf(
			"loan assessment authorization failed: %w",
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
		return nil, fmt.Errorf("loan application not found")
	}

	if app.Id != cmd.ApplicationID {
		return nil, fmt.Errorf("loan application ID mismatch")
	}

	// 5. Chỉ thẩm định hồ sơ under_review.
	if app.Status != loandomain.ApplicationStatusUnderReview {
		return nil, loandomain.ErrInvalidApplicationStatus
	}

	// Không ghi lại assessment đã có.
	if app.AssessedAt != nil {
		return nil, fmt.Errorf(
			"loan application has already been assessed",
		)
	}

	// 6. Lấy dữ liệu từ nguồn đáng tin cậy.
	//
	// DataProvider phải tạo Affordability và
	// LoanAssessmentInput theo chính sách hiện tại.
	input, err := h.data.BuildAssessmentInput(ctx, app)
	if err != nil {
		return nil, fmt.Errorf(
			"build loan assessment input failed: %w",
			err,
		)
	}

	// Assessment đã ghi sẽ khóa cập nhật evidence và không được chạy lại.
	// Giữ hồ sơ chưa assessed khi còn thiếu xác minh để nhân viên bổ sung
	// evidence rồi thử lại; manual_review vẫn dành cho dữ liệu đã xác minh
	// nhưng cần quyết định thủ công theo chính sách (DTI, lịch sử trả chậm...).
	if !input.FinancialDataVerified ||
		!input.DocumentsVerified ||
		!input.InsuranceVerified ||
		!input.HistoryVerified {
		return nil, fmt.Errorf("loan assessment requires complete verified evidence")
	}

	// 7. Gọi Domain để chấm điểm.
	preview := *app

	result, err := preview.ApplyAssessment(input)
	if err != nil {
		return nil, fmt.Errorf(
			"assess loan application failed: %w",
			err,
		)
	}
	preview.AssessedBy = cmd.ActorID
	if preview.AssessedAt == nil {
		return nil, fmt.Errorf(
			"loan assessment timestamp is missing",
		)
	}

	// 8. Chuẩn bị sự kiện chấm điểm.
	assessmentPayload := struct {
		DTIScore       int                               `json:"dtiScore"`
		InsuranceScore int                               `json:"insuranceScore"`
		HistoryScore   int                               `json:"historyScore"`
		TotalScore     int                               `json:"totalScore"`
		Decision       loandomain.LoanAssessmentDecision `json:"decision"`
		Reasons        []string                          `json:"reasons"`
		PolicyVersion  string                            `json:"policyVersion"`
		AssessedAt     time.Time                         `json:"assessedAt"`
		UpdatedAt      time.Time                         `json:"updatedAt"`
		AssessedBy     string                            `json:"assessedBy"`
	}{
		DTIScore:       result.DTIScore,
		InsuranceScore: result.InsuranceScore,
		HistoryScore:   result.HistoryScore,
		TotalScore:     result.TotalScore,
		Decision:       result.Decision,
		Reasons:        result.Reasons,
		PolicyVersion:  loanAssessmentPolicyVersion,
		AssessedAt:     *preview.AssessedAt,
		UpdatedAt:      preview.UpdatedAt,
		AssessedBy:     cmd.ActorID,
	}

	events := []LoanAppendEvent{
		{
			EventType: loanApplicationAssessedEvent,
			Payload:   assessmentPayload,
		},
	}

	// 9. Bổ sung sự kiện quyết định tự động nếu cần.
	var decisionEvent string
	var decisionAt time.Time

	switch result.Decision {

	case loandomain.AssessmentAutoEligible:
		if preview.ApprovedAt == nil {
			return nil, fmt.Errorf(
				"automatic approval timestamp is missing",
			)
		}

		decisionEvent = loanApplicationApprovedEvent
		decisionAt = *preview.ApprovedAt

	case loandomain.AssessmentRejected:
		if preview.RejectedAt == nil {
			return nil, fmt.Errorf(
				"automatic rejection timestamp is missing",
			)
		}

		decisionEvent = loanApplicationRejectedEvent
		decisionAt = *preview.RejectedAt

	case loandomain.AssessmentManualReview:
		// Chỉ ghi assessment.
		// Hồ sơ tiếp tục chờ nhân viên quyết định.

	default:
		return nil, fmt.Errorf(
			"unsupported loan assessment decision: %s",
			result.Decision,
		)
	}

	if decisionEvent != "" {
		decisionPayload := struct {
			DecidedAt time.Time `json:"decidedAt"`
			UpdatedAt time.Time `json:"updatedAt"`
		}{
			DecidedAt: decisionAt,
			UpdatedAt: preview.UpdatedAt,
		}

		events = append(
			events,
			LoanAppendEvent{
				EventType: decisionEvent,
				Payload:   decisionPayload,
			},
		)
	}

	// 10. Ghi sự kiện nguyên tử vào EventStoreDB.
	//
	// Auto approve/reject: 2 events.
	// Manual review: 1 event.
	if err := h.repo.AppendBatch(
		ctx,
		app.Id,
		revision,
		events...,
	); err != nil {
		return nil, fmt.Errorf(
			"append loan assessment events failed: %w",
			err,
		)
	}

	// 11. Chỉ trả kết quả sau khi ghi thành công.
	return &preview, nil
}
