package commands

import (
	"context"

	"github.com/dont-wait/anomaly/internal/application/loan/ports"
	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
)

// LoanApplicationRepository định nghĩa những thao tác
// lưu và đọc hồ sơ vay mà Application cần sử dụng.
//
// Implementation thực tế sẽ nằm ở Infrastructure,
// sử dụng EventStoreDB làm nguồn dữ liệu chính.
type LoanApplicationRepository interface {
	// Create ghi sự kiện LoanApplicationCreated.
	Create(
		ctx context.Context,
		app *loandomain.Application,
	) error

	// FindByID khôi phục hồ sơ từ lịch sử sự kiện.
	FindByID(
		ctx context.Context,
		applicationID string,
	) (*loandomain.Application, error)
}

// LoanApplicantValidator kiểm tra điều kiện khách hàng
// và tài khoản nhận giải ngân trước khi tạo hồ sơ.
//
// Implementation phải dùng dữ liệu đáng tin cậy,
// không chỉ dựa vào thông tin do request cung cấp.
type LoanApplicantValidator interface {
	ValidateDisbursementAccount(
		ctx context.Context,
		customerID string,
		disbursementAccountID string,
	) error
}

// LoanDocumentRepository cung cấp các thao tác
// cần thiết để cập nhật chứng từ bằng Event Sourcing.
type LoanDocumentRepository interface {
	FindByIDWithRevision(
		ctx context.Context,
		applicationID string,
	) (*loandomain.Application, uint64, error)

	Append(
		ctx context.Context,
		applicationID string,
		expectedRevision uint64,
		eventType string,
		payload any,
	) error
}

// LoanDocumentStorageVerifier xác minh chứng từ
// đã tồn tại trên RustFS và thuộc đúng khách hàng.
//
// Implementation thực tế phải đối chiếu metadata
// với đối tượng đang lưu trữ, không tin dữ liệu
// do client tự khai báo.
type LoanDocumentStorageVerifier interface {
	VerifyLoanDocument(
		ctx context.Context,
		customerID string,
		applicationID string,
		document loandomain.ApplicationDocument,
	) error
}

// LoanReviewAuthorizer kiểm tra nhân viên hoặc
// chủ thể hệ thống có quyền bắt đầu thẩm định.
//
// Implementation thực tế phải xác minh danh tính
// và quyền từ nguồn đáng tin cậy.
type LoanReviewAuthorizer interface {
	AuthorizeLoanReview(
		ctx context.Context,
		actorID string,
	) error
}

// LoanAppendEvent biểu diễn một sự kiện cần ghi
// vào EventStoreDB thông qua Repository.
type LoanAppendEvent = ports.LoanAppendEvent

// LoanAssessmentRepository cung cấp thao tác đọc hồ sơ
// và ghi nhiều sự kiện trong một lần AppendBatch.
type LoanAssessmentRepository interface {
	FindByIDWithRevision(
		ctx context.Context,
		applicationID string,
	) (*loandomain.Application, uint64, error)

	AppendBatch(
		ctx context.Context,
		applicationID string,
		expectedRevision uint64,
		events ...LoanAppendEvent,
	) error
}

// LoanAssessmentAuthorizer kiểm tra quyền chấm điểm,
// sử dụng danh tính chủ thể đã xác thực.
type LoanAssessmentAuthorizer interface {
	AuthorizeLoanAssessment(
		ctx context.Context,
		actorID string,
	) error
}

// LoanAssessmentDataProvider tạo dữ liệu đầu vào
// cho thuật toán chấm điểm từ nguồn đáng tin cậy.
//
// Implementation thực tế phải xác minh thu nhập,
// khoản nợ hiện tại, BHXH, lịch sử tín dụng,
// chứng từ và tính toán Affordability.
//
// Không nhận trực tiếp các cờ Verified hoặc
// điểm số tự khai báo từ HTTP request.
type LoanAssessmentDataProvider interface {
	BuildAssessmentInput(
		ctx context.Context,
		app *loandomain.Application,
	) (loandomain.LoanAssessmentInput, error)
}

// LoanManualDecisionAuthorizer kiểm tra quyền
// phê duyệt hoặc từ chối hồ sơ vay thủ công.
//
// Implementation thực tế phải xác minh quyền
// của nhân viên từ hệ thống xác thực.
type LoanManualDecisionAuthorizer interface {
	AuthorizeLoanManualDecision(
		ctx context.Context,
		actorID string,
	) error
}
