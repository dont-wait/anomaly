package eventstore

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
)

const (
	EventLoanApplicationCreated   = "LoanApplicationCreated"
	EventLoanDocumentAdded        = "LoanDocumentAdded"
	EventLoanApplicationSubmitted = "LoanApplicationSubmitted"
	EventLoanReviewStarted        = "LoanReviewStarted"
	EventLoanApplicationAssessed  = "LoanApplicationAssessed"
	EventLoanApplicationApproved  = "LoanApplicationApproved"
	EventLoanApplicationRejected  = "LoanApplicationRejected"
	EventLoanDisbursementPrepared = "LoanDisbursementPrepared"
	EventLoanOfferIssued          = "LoanOfferIssued"
	EventLoanOfferAccepted        = "LoanOfferAccepted"
	EventLoanOfferRejected        = "LoanOfferRejected"
	EventLoanContractConfirmed    = "LoanContractConfirmed"
	EventLoanDisbursed            = "LoanDisbursed"
)

type loanApplicationCreatedPayload struct {
	Application *loandomain.Application `json:"application"`
}

type loanDocumentAddedPayload struct {
	Document  loandomain.ApplicationDocument `json:"document"`
	UpdatedAt time.Time                      `json:"updatedAt"`
}

type loanApplicationSubmittedPayload struct {
	SubmittedAt time.Time `json:"submittedAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type loanReviewStartedPayload struct {
	ReviewStartedAt time.Time `json:"reviewStartedAt"`
	ReviewerId      string    `json:"reviewerId,omitempty"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type loanApplicationAssessedPayload struct {
	DTIScore       int                               `json:"dtiScore"`
	InsuranceScore int                               `json:"insuranceScore"`
	HistoryScore   int                               `json:"historyScore"`
	TotalScore     int                               `json:"totalScore"`
	Decision       loandomain.LoanAssessmentDecision `json:"decision"`
	Reasons        []string                          `json:"reasons"`
	PolicyVersion  string                            `json:"policyVersion"`
	AssessedAt     time.Time                         `json:"assessedAt"`
	UpdatedAt      time.Time                         `json:"updatedAt"`
	AssessedBy     string                            `json:"assessedBy,omitempty"`
}

type loanDecisionPayload struct {
	DecidedAt            time.Time  `json:"decidedAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
	ManualReviewerId     string     `json:"manualReviewerId,omitempty"`
	ManualDecisionReason string     `json:"manualDecisionReason,omitempty"`
	ManualDecidedAt      *time.Time `json:"manualDecidedAt,omitempty"`
}
type loanDisbursementPreparedPayload struct {
	PreparedAt    time.Time `json:"preparedAt"`
	AnnualRateBps int64     `json:"annualRateBps"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type loanDisbursedPayload struct {
	DisbursedAt       time.Time                          `json:"disbursedAt"`
	AnnualRateBps     int64                              `json:"annualRateBps"`
	RepaymentSchedule []loandomain.RepaymentScheduleItem `json:"repaymentSchedule"`
	UpdatedAt         time.Time                          `json:"updatedAt"`
}

type loanOfferIssuedPayload struct {
	IssuedAt      time.Time `json:"issuedAt"`
	AnnualRateBps int64     `json:"annualRateBps"`
	UpdatedAt     time.Time `json:"updatedAt"`
}
type loanOfferTransitionPayload struct {
	At        time.Time `json:"at"`
	UpdatedAt time.Time `json:"updatedAt"`
	Method    string    `json:"method,omitempty"`
}

func loanStreamName(applicationID string) string {
	return fmt.Sprintf("loan-application-%s", applicationID)
}

// applyLoanEvent áp dụng một sự kiện để khôi phục hồ sơ vay.
// Việc kiểm tra nghiệp vụ có thể dùng Domain,
// nhưng thời gian cuối cùng luôn lấy từ event đã lưu.
func applyLoanEvent(
	app *loandomain.Application,
	eventType string,
	data []byte,
) error {
	if app == nil {
		return fmt.Errorf("loan application is nil")
	}

	if eventType != EventLoanApplicationCreated && app.Id == "" {
		return fmt.Errorf("loan application has not been created")
	}

	switch eventType {

	case EventLoanApplicationCreated:
		if app.Id != "" {
			return fmt.Errorf("loan application already created")
		}

		var p loanApplicationCreatedPayload
		if err := json.Unmarshal(data, &p); err != nil {
			return err
		}

		if p.Application == nil {
			return fmt.Errorf("invalid loan creation payload")
		}

		source := p.Application

		if source.Status != loandomain.ApplicationStatusDraft {
			return fmt.Errorf("new loan must be in draft status")
		}

		if len(source.Documents) != 0 {
			return fmt.Errorf("new loan must not contain documents")
		}

		if source.CreatedAt.IsZero() || source.UpdatedAt.IsZero() {
			return fmt.Errorf("loan creation timestamps are required")
		}

		if source.UpdatedAt.Before(source.CreatedAt) {
			return fmt.Errorf("invalid loan creation timestamps")
		}

		// Sử dụng Domain để kiểm tra dữ liệu tạo hồ sơ.
		// Không dùng timestamp do constructor sinh ra.
		if _, err := loandomain.NewDraftApplication(
			source.Id,
			source.ApplicationNo,
			source.CustomerId,
			source.LoanPackageId,
			source.DisbursementAccountId,
			source.RequestedAmount,
			source.TermMonths,
		); err != nil {
			return fmt.Errorf("invalid loan creation: %w", err)
		}

		*app = *source

	case EventLoanDocumentAdded:
		var p loanDocumentAddedPayload
		if err := json.Unmarshal(data, &p); err != nil {
			return err
		}

		if p.Document.UploadedAt.IsZero() || p.UpdatedAt.IsZero() {
			return fmt.Errorf("document timestamps are required")
		}

		if p.UpdatedAt.Before(p.Document.UploadedAt) {
			return fmt.Errorf("invalid document timestamps")
		}

		if p.Document.VerificationStatus != "pending" {
			return fmt.Errorf("new document must be pending")
		}

		// Sao chép trạng thái để kiểm tra bằng Domain.
		// Không làm thay đổi app nếu validation thất bại.
		preview := *app
		preview.Documents = append(
			[]loandomain.ApplicationDocument(nil),
			app.Documents...,
		)

		if err := preview.AddDocument(p.Document); err != nil {
			return fmt.Errorf("invalid document event: %w", err)
		}

		// Ghi dữ liệu và thời gian gốc từ event.
		app.Documents = append(app.Documents, p.Document)
		app.UpdatedAt = p.UpdatedAt

	case EventLoanApplicationSubmitted:
		var p loanApplicationSubmittedPayload
		if err := json.Unmarshal(data, &p); err != nil {
			return err
		}

		if p.SubmittedAt.IsZero() || p.UpdatedAt.IsZero() {
			return fmt.Errorf("submission timestamps are required")
		}

		if p.SubmittedAt.Before(app.CreatedAt) ||
			p.UpdatedAt.Before(p.SubmittedAt) {
			return fmt.Errorf("invalid submission timestamps")
		}

		// Domain kiểm tra trạng thái draft và 7 loại chứng từ.
		preview := *app
		if err := preview.Submit(); err != nil {
			return fmt.Errorf("cannot submit loan: %w", err)
		}

		app.Status = loandomain.ApplicationStatusSubmitted
		app.SubmittedAt = &p.SubmittedAt
		app.UpdatedAt = p.UpdatedAt

	case EventLoanReviewStarted:
		var p loanReviewStartedPayload
		if err := json.Unmarshal(data, &p); err != nil {
			return err
		}

		if p.ReviewStartedAt.IsZero() || p.UpdatedAt.IsZero() {
			return fmt.Errorf("review timestamps are required")
		}

		if app.SubmittedAt == nil ||
			p.ReviewStartedAt.Before(*app.SubmittedAt) ||
			p.UpdatedAt.Before(p.ReviewStartedAt) {
			return fmt.Errorf("invalid review timestamps")
		}

		preview := *app
		if err := preview.StartReview(); err != nil {
			return fmt.Errorf("cannot start loan review: %w", err)
		}

		app.Status = loandomain.ApplicationStatusUnderReview
		app.ReviewStartedAt = &p.ReviewStartedAt
		app.ReviewStartedBy = p.ReviewerId
		app.UpdatedAt = p.UpdatedAt

	case EventLoanApplicationAssessed:
		if app.Status != loandomain.ApplicationStatusUnderReview {
			return fmt.Errorf("cannot assess in status %s", app.Status)
		}

		if app.AssessedAt != nil {
			return fmt.Errorf("loan application already assessed")
		}

		var p loanApplicationAssessedPayload
		if err := json.Unmarshal(data, &p); err != nil {
			return err
		}

		if err := validateLoanAssessmentPayload(p); err != nil {
			return err
		}

		if app.ReviewStartedAt == nil ||
			p.AssessedAt.Before(*app.ReviewStartedAt) ||
			p.UpdatedAt.Before(p.AssessedAt) {
			return fmt.Errorf("invalid assessment timestamps")
		}

		app.AssessmentScore = p.TotalScore
		app.AssessmentDecision = p.Decision
		app.AssessmentReasons = append(
			[]string(nil),
			p.Reasons...,
		)
		app.AssessedAt = &p.AssessedAt
		app.UpdatedAt = p.UpdatedAt
		app.AssessedBy = p.AssessedBy
		// Chỉ ghi nhận kết quả đánh giá.
		// Chưa chuyển sang approved hoặc rejected.

	case EventLoanApplicationApproved,
		EventLoanApplicationRejected:
		var p loanDecisionPayload
		if err := json.Unmarshal(data, &p); err != nil {
			return err
		}

		if err := applyLoanDecisionEvent(
			app,
			eventType,
			p,
		); err != nil {
			return err
		}

	case EventLoanOfferIssued:
		var p loanOfferIssuedPayload
		if err := json.Unmarshal(data, &p); err != nil {
			return err
		}
		if p.UpdatedAt.IsZero() || !p.UpdatedAt.Equal(p.IssuedAt) {
			return fmt.Errorf("invalid offer timestamps")
		}
		preview := *app
		if err := preview.IssueOffer(p.IssuedAt, p.AnnualRateBps); err != nil {
			return err
		}
		*app = preview

	case EventLoanOfferAccepted, EventLoanOfferRejected, EventLoanContractConfirmed:
		var p loanOfferTransitionPayload
		if err := json.Unmarshal(data, &p); err != nil {
			return err
		}
		if p.UpdatedAt.IsZero() || !p.UpdatedAt.Equal(p.At) {
			return fmt.Errorf("invalid offer transition timestamps")
		}
		preview := *app
		if app.Offer != nil {
			offer := *app.Offer
			preview.Offer = &offer
		}
		var err error
		switch eventType {
		case EventLoanOfferAccepted:
			if p.Method != "" {
				return fmt.Errorf("unexpected offer acceptance method")
			}
			err = preview.AcceptOffer(p.At)
		case EventLoanOfferRejected:
			if p.Method != "" {
				return fmt.Errorf("unexpected offer rejection method")
			}
			err = preview.RejectOffer(p.At)
		case EventLoanContractConfirmed:
			err = preview.ConfirmContract(p.At, p.Method)
		}
		if err != nil {
			return err
		}
		*app = preview

	case EventLoanDisbursementPrepared:
		var p loanDisbursementPreparedPayload

		if err := json.Unmarshal(data, &p); err != nil {
			return err
		}

		if p.UpdatedAt.IsZero() ||
			!p.UpdatedAt.Equal(p.PreparedAt) {
			return fmt.Errorf(
				"invalid disbursement preparation timestamps",
			)
		}

		preview := *app

		if err := preview.PrepareDisbursement(
			p.PreparedAt,
			p.AnnualRateBps,
		); err != nil {
			return fmt.Errorf(
				"cannot prepare loan disbursement: %w",
				err,
			)
		}

		*app = preview

	case EventLoanDisbursed:
		var p loanDisbursedPayload

		if err := json.Unmarshal(data, &p); err != nil {
			return err
		}

		if p.UpdatedAt.IsZero() ||
			!p.UpdatedAt.Equal(p.DisbursedAt) {
			return fmt.Errorf("invalid disbursement timestamps")
		}

		preview := *app

		if err := preview.MarkDisbursed(
			p.DisbursedAt,
			p.AnnualRateBps,
		); err != nil {
			return err
		}

		if !reflect.DeepEqual(
			preview.RepaymentSchedule,
			p.RepaymentSchedule,
		) {
			return fmt.Errorf(
				"disbursement repayment schedule mismatch",
			)
		}

		*app = preview
	default:
		return fmt.Errorf(
			"unknown loan event type: %s",
			eventType,
		)
	}

	return nil
}

// Kiểm tra tính nhất quán cơ bản của kết quả chấm điểm.
// Không thay thế việc chạy AssessLoanApplication
// với dữ liệu đã xác minh ở Application Layer.
func validateLoanAssessmentPayload(
	p loanApplicationAssessedPayload,
) error {
	if strings.TrimSpace(p.PolicyVersion) == "" {
		return fmt.Errorf("loan assessment policy version is required")
	}

	if p.AssessedAt.IsZero() || p.UpdatedAt.IsZero() {
		return fmt.Errorf("assessment timestamps are required")
	}

	validDTI := p.DTIScore == 0 ||
		p.DTIScore == 15 ||
		p.DTIScore == 30 ||
		p.DTIScore == 40

	validInsurance := p.InsuranceScore == 0 ||
		p.InsuranceScore == 10 ||
		p.InsuranceScore == 20 ||
		p.InsuranceScore == 25

	validHistory := p.HistoryScore == 0 ||
		p.HistoryScore == 15 ||
		p.HistoryScore == 30 ||
		p.HistoryScore == 35

	if !validDTI || !validInsurance || !validHistory {
		return fmt.Errorf("invalid loan assessment component scores")
	}

	sum := p.DTIScore + p.InsuranceScore + p.HistoryScore
	if p.TotalScore != sum {
		return fmt.Errorf(
			"assessment total score mismatch: expected %d, got %d",
			sum,
			p.TotalScore,
		)
	}

	switch p.Decision {
	case loandomain.AssessmentAutoEligible:
		// Các mức điểm dưới đây luôn phát sinh manual review
		// hoặc không đạt ngưỡng tự động theo chính sách hiện tại.
		if p.TotalScore < 80 ||
			p.DTIScore < 30 ||
			p.InsuranceScore < 20 ||
			p.HistoryScore < 30 ||
			len(p.Reasons) > 0 {
			return fmt.Errorf(
				"inconsistent automatic assessment decision",
			)
		}

	case loandomain.AssessmentManualReview:
		// Cho phép tổng điểm bất kỳ theo kết quả Domain.

	case loandomain.AssessmentRejected:
		if len(p.Reasons) == 0 {
			return fmt.Errorf("rejection assessment reason is required")
		}

	default:
		return fmt.Errorf(
			"invalid loan assessment decision: %s",
			p.Decision,
		)
	}

	return nil
}

// Kiểm tra và áp dụng quyết định cuối cùng.
// Dùng Domain để xác thực quyết định thủ công.
func applyLoanDecisionEvent(
	app *loandomain.Application,
	eventType string,
	p loanDecisionPayload,
) error {
	if app.Status != loandomain.ApplicationStatusUnderReview {
		return fmt.Errorf(
			"cannot finalize loan in status %s",
			app.Status,
		)
	}

	if app.AssessedAt == nil {
		return fmt.Errorf("loan must be assessed before decision")
	}

	if p.DecidedAt.IsZero() || p.UpdatedAt.IsZero() {
		return fmt.Errorf("decision timestamps are required")
	}

	if p.DecidedAt.Before(*app.AssessedAt) ||
		p.UpdatedAt.Before(p.DecidedAt) {
		return fmt.Errorf("invalid decision timestamps")
	}

	targetStatus := loandomain.ApplicationStatusApproved
	if eventType == EventLoanApplicationRejected {
		targetStatus = loandomain.ApplicationStatusRejected
	}

	if app.AssessmentDecision == loandomain.AssessmentManualReview {
		if strings.TrimSpace(p.ManualReviewerId) == "" ||
			strings.TrimSpace(p.ManualDecisionReason) == "" ||
			p.ManualDecidedAt == nil ||
			p.ManualDecidedAt.IsZero() {
			return fmt.Errorf(
				"manual decision requires reviewer, reason and timestamp",
			)
		}

		if !p.ManualDecidedAt.Equal(p.DecidedAt) {
			return fmt.Errorf("manual decision timestamps do not match")
		}

		// Tận dụng kiểm tra nghiệp vụ trong Domain.
		preview := *app
		if err := preview.ApplyManualDecision(
			p.ManualReviewerId,
			targetStatus,
			p.ManualDecisionReason,
		); err != nil {
			return fmt.Errorf("invalid manual decision: %w", err)
		}

	} else {
		// Quyết định tự động không được giả danh thao tác nhân viên.
		if p.ManualReviewerId != "" ||
			p.ManualDecisionReason != "" ||
			p.ManualDecidedAt != nil {
			return fmt.Errorf(
				"automatic decision must not contain manual reviewer",
			)
		}

		if eventType == EventLoanApplicationApproved &&
			app.AssessmentDecision != loandomain.AssessmentAutoEligible {
			return fmt.Errorf(
				"loan is not eligible for automatic approval",
			)
		}

		if eventType == EventLoanApplicationRejected &&
			app.AssessmentDecision != loandomain.AssessmentRejected {
			return fmt.Errorf(
				"loan is not eligible for automatic rejection",
			)
		}
	}

	app.Status = targetStatus
	app.UpdatedAt = p.UpdatedAt
	app.ManualReviewerId = p.ManualReviewerId
	app.ManualDecisionReason = p.ManualDecisionReason
	app.ManualDecidedAt = p.ManualDecidedAt

	if targetStatus == loandomain.ApplicationStatusApproved {
		app.ApprovedAt = &p.DecidedAt
	} else {
		app.RejectedAt = &p.DecidedAt
	}

	return nil
}
