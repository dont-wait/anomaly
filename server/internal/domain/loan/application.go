package loan

import (
	"fmt"
	"strings"
	"time"
)

type ApplicationDocument struct {
	DocumentId         string    `json:"documentId"`
	Type               string    `json:"type"`
	StorageKey         string    `json:"storageKey"`
	MIMEType           string    `json:"mimeType"`
	SizeBytes          int64     `json:"sizeBytes"`
	SHA256             string    `json:"sha256"`
	VerificationStatus string    `json:"verificationStatus"`
	UploadedAt         time.Time `json:"uploadedAt"`
}

// ApplicationStatus là trạng thái của hồ sơ vay.
type ApplicationStatus string

const (
	ApplicationStatusDraft       ApplicationStatus = "draft"
	ApplicationStatusSubmitted   ApplicationStatus = "submitted"
	ApplicationStatusUnderReview ApplicationStatus = "under_review"
	ApplicationStatusApproved    ApplicationStatus = "approved"
	ApplicationStatusRejected    ApplicationStatus = "rejected"
	ApplicationStatusDisbursed   ApplicationStatus = "disbursed"
)

// Application đại diện cho một hồ sơ đăng ký vay vốn cá nhân.

type Application struct {
	Id                     string                  `json:"id"`
	ApplicationNo          string                  `json:"applicationNo"`
	CustomerId             string                  `json:"customerId"`
	LoanPackageId          string                  `json:"loanPackageId"`
	DisbursementAccountId  string                  `json:"disbursementAccountId"`
	RequestedAmount        int64                   `json:"requestedAmount"`
	TermMonths             int                     `json:"termMonths"`
	Documents              []ApplicationDocument   `json:"documents"`
	Status                 ApplicationStatus       `json:"status"`
	CreatedAt              time.Time               `json:"createdAt"`
	UpdatedAt              time.Time               `json:"updatedAt"`
	SubmittedAt            *time.Time              `json:"submittedAt"`
	ReviewStartedAt        *time.Time              `json:"reviewStartedAt"`
	AssessmentScore        int                     `json:"assessmentScore"`
	AssessmentDecision     LoanAssessmentDecision  `json:"assessmentDecision"`
	AssessmentReasons      []string                `json:"assessmentReasons"`
	AssessedAt             *time.Time              `json:"assessedAt"`
	ApprovedAt             *time.Time              `json:"approvedAt"`
	RejectedAt             *time.Time              `json:"rejectedAt"`
	ManualReviewerId       string                  `json:"manualReviewerId"`
	ManualDecisionReason   string                  `json:"manualDecisionReason"`
	ManualDecidedAt        *time.Time              `json:"manualDecidedAt"`
	ReviewStartedBy        string                  `json:"reviewStartedBy"`
	AssessedBy             string                  `json:"assessedBy"`
	DisbursedAt            *time.Time              `json:"disbursedAt"`
	AnnualRateBps          int64                   `json:"annualRateBps"`
	DisbursementPreparedAt *time.Time              `json:"disbursementPreparedAt"`
	PreparedAnnualRateBps  int64                   `json:"preparedAnnualRateBps"`
	RepaymentSchedule      []RepaymentScheduleItem `json:"repaymentSchedule"`
}

func NewDraftApplication(
	id string,
	applicationNo string,
	customerId string,
	loanPackageId string,
	disbursementAccountId string,
	requestedAmount int64,
	termMonths int,
) (*Application, error) {

	// Kiểm tra các định danh bắt buộc.
	if strings.TrimSpace(id) == "" ||
		strings.TrimSpace(applicationNo) == "" ||
		strings.TrimSpace(customerId) == "" {
		return nil, fmt.Errorf("application and customer identifiers are required")
	}

	// Kiểm tra gói vay.
	if strings.TrimSpace(loanPackageId) == "" {
		return nil, ErrInvalidLoanPackage
	}

	// Kiểm tra tài khoản nhận giải ngân.
	if strings.TrimSpace(disbursementAccountId) == "" {
		return nil, ErrInvalidDisbursementAccount
	}

	// Kiểm tra số tiền vay.
	if requestedAmount <= 0 {
		return nil, ErrInvalidAmount
	}

	// Kiểm tra kỳ hạn hợp lệ về mặt dữ liệu.
	if termMonths <= 0 {
		return nil, ErrUnsupportedTerm
	}

	now := time.Now().UTC()

	application := &Application{
		Id:                    id,
		ApplicationNo:         applicationNo,
		CustomerId:            customerId,
		LoanPackageId:         loanPackageId,
		DisbursementAccountId: disbursementAccountId,
		RequestedAmount:       requestedAmount,
		TermMonths:            termMonths,
		Documents:             []ApplicationDocument{},
		Status:                ApplicationStatusDraft,
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	return application, nil
}

func (a *Application) AddDocument(document ApplicationDocument) error {
	// Kiểm tra hồ sơ tồn tại.
	if a == nil {
		return fmt.Errorf("loan application is nil")
	}

	// Chỉ cho phép thêm chứng từ khi hồ sơ đang là bản nháp.
	if a.Status != ApplicationStatusDraft {
		return ErrInvalidApplicationStatus
	}

	// Kiểm tra thông tin chứng từ.
	if strings.TrimSpace(document.DocumentId) == "" ||
		strings.TrimSpace(document.Type) == "" ||
		strings.TrimSpace(document.StorageKey) == "" ||
		strings.TrimSpace(document.MIMEType) == "" ||
		strings.TrimSpace(document.SHA256) == "" ||
		document.SizeBytes <= 0 {
		return fmt.Errorf("invalid application document metadata")
	}

	// Không cho phép trùng mã chứng từ trong cùng hồ sơ.
	for _, existing := range a.Documents {
		if existing.DocumentId == document.DocumentId {
			return fmt.Errorf("document ID already exists")
		}
	}

	// Gán trạng thái kiểm tra ban đầu.
	document.VerificationStatus = "pending"
	document.UploadedAt = time.Now().UTC()

	// Thêm chứng từ vào hồ sơ.
	a.Documents = append(a.Documents, document)

	// Cập nhật thời gian chỉnh sửa hồ sơ.
	a.UpdatedAt = document.UploadedAt

	return nil
}

type MissingLoanDocumentsError struct {
	Types []string
}

func (e *MissingLoanDocumentsError) Error() string {
	return "missing required documents: " +
		strings.Join(e.Types, ", ")
}

// Submit chuyển hồ sơ từ draft sang submitted.
func (a *Application) Submit() error {
	if a == nil {
		return fmt.Errorf("loan application is nil")
	}

	// Chỉ hồ sơ nháp mới được gửi thẩm định.
	if a.Status != ApplicationStatusDraft {
		return ErrInvalidApplicationStatus
	}

	// Danh sách chứng từ bắt buộc.
	requiredTypes := []string{
		"IDENTITY_FRONT",
		"IDENTITY_BACK",
		"VNEID_PERSONAL",
		"VNEID_RESIDENCE",
		"VSSID_PARTICIPATION",
		"VSSID_DETAIL",
		"SALARY_STATEMENT_3_MONTHS",
	}

	// Tổng hợp những loại chứng từ đã tải lên.
	uploaded := make(map[string]bool)

	for _, doc := range a.Documents {
		if doc.VerificationStatus != "rejected" {
			uploaded[doc.Type] = true
		}
	}

	// Kiểm tra chứng từ còn thiếu.
	var missing []string

	for _, required := range requiredTypes {
		if !uploaded[required] {
			missing = append(missing, required)
		}
	}

	if len(missing) > 0 {
		return &MissingLoanDocumentsError{
			Types: missing,
		}
	}

	// Gửi hồ sơ thành công.
	now := time.Now().UTC()

	a.Status = ApplicationStatusSubmitted
	a.SubmittedAt = &now
	a.UpdatedAt = now

	return nil
}

func (a *Application) StartReview() error {
	// Kiểm tra hồ sơ tồn tại.
	if a == nil {
		return fmt.Errorf("loan application is nil")
	}

	// Chỉ hồ sơ đã submitted mới được chuyển sang thẩm định.
	if a.Status != ApplicationStatusSubmitted {
		return ErrInvalidApplicationStatus
	}

	now := time.Now().UTC()

	// Cập nhật trạng thái hồ sơ.
	a.Status = ApplicationStatusUnderReview
	a.ReviewStartedAt = &now
	a.UpdatedAt = now

	return nil
}

func (a *Application) ApplyAssessment(
	input LoanAssessmentInput,
) (*LoanAssessmentResult, error) {

	// 1. Kiểm tra hồ sơ tồn tại.
	if a == nil {
		return nil, fmt.Errorf("loan application is nil")
	}

	// 2. Chỉ hồ sơ đang thẩm định mới được đánh giá.
	if a.Status != ApplicationStatusUnderReview {
		return nil, ErrInvalidApplicationStatus
	}

	// 3. Chấm điểm theo quy tắc nội bộ.
	result, err := AssessLoanApplication(input)

	if err != nil {
		return nil, err
	}

	// 4. Ghi nhận kết quả thẩm định.
	now := time.Now().UTC()

	a.AssessmentScore = result.TotalScore
	a.AssessmentDecision = result.Decision
	a.AssessmentReasons = append(
		[]string{},
		result.Reasons...,
	)
	a.AssessedAt = &now
	a.UpdatedAt = now

	// 5. Xử lý trạng thái theo kết quả.
	switch result.Decision {

	case AssessmentAutoEligible:
		// Đủ điều kiện phê duyệt tự động.
		a.Status = ApplicationStatusApproved
		a.ApprovedAt = &now

	case AssessmentRejected:
		// Không đạt điều kiện thẩm định.
		a.Status = ApplicationStatusRejected
		a.RejectedAt = &now

	case AssessmentManualReview:
		// Giữ nguyên trạng thái để nhân viên xem xét.
		a.Status = ApplicationStatusUnderReview

	default:
		return nil, fmt.Errorf(
			"unsupported assessment decision: %s",
			result.Decision,
		)
	}

	return result, nil
}

func (a *Application) ApplyManualDecision(
	reviewerId string,
	decision ApplicationStatus,
	reason string,
) error {

	// 1. Kiểm tra hồ sơ tồn tại.
	if a == nil {
		return fmt.Errorf("loan application is nil")
	}

	// 2. Hồ sơ phải đang được thẩm định.
	if a.Status != ApplicationStatusUnderReview {
		return ErrInvalidApplicationStatus
	}

	// 3. Chỉ xử lý thủ công khi hệ thống đã phân luồng.
	if a.AssessmentDecision != AssessmentManualReview ||
		a.AssessedAt == nil {
		return fmt.Errorf(
			"application is not awaiting manual decision",
		)
	}

	// 4. Kiểm tra mã nhân viên.
	reviewerId = strings.TrimSpace(reviewerId)

	if reviewerId == "" {
		return fmt.Errorf("reviewer ID is required")
	}

	// 5. Bắt buộc ghi lý do.
	reason = strings.TrimSpace(reason)

	if reason == "" {
		return fmt.Errorf("manual decision reason is required")
	}

	// 6. Chỉ chấp nhận approved hoặc rejected.
	if decision != ApplicationStatusApproved &&
		decision != ApplicationStatusRejected {
		return fmt.Errorf("invalid manual decision")
	}

	// 7. Ghi nhận quyết định.
	now := time.Now().UTC()

	a.Status = decision
	a.ManualReviewerId = reviewerId
	a.ManualDecisionReason = reason
	a.ManualDecidedAt = &now
	a.UpdatedAt = now

	// 8. Ghi nhận thời điểm phê duyệt hoặc từ chối.
	switch decision {
	case ApplicationStatusApproved:
		a.ApprovedAt = &now

	case ApplicationStatusRejected:
		a.RejectedAt = &now
	}

	return nil
}

func (a *Application) PrepareDisbursement(
	at time.Time,
	annualRateBps int64,
) error {
	if a == nil ||
		a.Status != ApplicationStatusApproved ||
		a.ApprovedAt == nil {
		return ErrInvalidApplicationStatus
	}

	if at.IsZero() ||
		at.Before(*a.ApprovedAt) ||
		annualRateBps <= 0 {
		return fmt.Errorf("invalid loan disbursement preparation")
	}

	// Không cho phép chốt lại điều khoản đã chuẩn bị.
	if a.DisbursementPreparedAt != nil {
		return fmt.Errorf("loan disbursement already prepared")
	}

	// Kiểm tra có thể tạo lịch trả nợ với điều khoản này.
	// Lịch chính thức vẫn tính từ thời điểm tiền được cộng.
	_, err := CalculateRepaymentSchedule(
		a.RequestedAmount,
		annualRateBps,
		a.TermMonths,
		at,
	)
	if err != nil {
		return err
	}

	a.DisbursementPreparedAt = &at
	a.PreparedAnnualRateBps = annualRateBps
	a.UpdatedAt = at

	return nil
}
