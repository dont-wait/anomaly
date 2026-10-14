package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
)

// CreateLoanApplicationCommand chứa dữ liệu
// cần thiết để đăng ký hồ sơ vay vốn.
type CreateLoanApplicationCommand struct {
	CustomerID            string
	LoanPackageID         string
	DisbursementAccountID string
	RequestedAmount       int64
	TermMonths            int
	IdempotencyKey        string
}

// CreateLoanApplicationCommandHandler điều phối
// nghiệp vụ tạo hồ sơ vay.
type CreateLoanApplicationCommandHandler struct {
	repo      LoanApplicationRepository
	validator LoanApplicantValidator
}

// Constructor sử dụng Dependency Injection.
func NewCreateLoanApplicationCommandHandler(
	repo LoanApplicationRepository,
	validator LoanApplicantValidator,
) *CreateLoanApplicationCommandHandler {
	return &CreateLoanApplicationCommandHandler{
		repo:      repo,
		validator: validator,
	}
}

// Handle tạo hồ sơ ở trạng thái draft
// và yêu cầu repository ghi sự kiện tạo hồ sơ.
func (h *CreateLoanApplicationCommandHandler) Handle(
	ctx context.Context,
	cmd CreateLoanApplicationCommand,
) (*loandomain.Application, error) {

	// 1. Kiểm tra dependency.
	if h == nil || h.repo == nil {
		return nil, fmt.Errorf("loan repository is not configured")
	}

	if h.validator == nil {
		return nil, fmt.Errorf("loan applicant validator is not configured")
	}

	// 2. Chuẩn hóa dữ liệu đầu vào.
	cmd.CustomerID = strings.TrimSpace(cmd.CustomerID)
	cmd.LoanPackageID = strings.TrimSpace(cmd.LoanPackageID)
	cmd.DisbursementAccountID = strings.TrimSpace(
		cmd.DisbursementAccountID,
	)

	// 3. Kiểm tra kỳ hạn theo chính sách vay.
	if !isSupportedLoanTerm(cmd.TermMonths) {
		return nil, loandomain.ErrUnsupportedTerm
	}

	// 4. Tạo định danh hồ sơ phía Server.
	applicationID := uuid.NewString()

	if cmd.IdempotencyKey != "" {
		key, err := uuid.Parse(
			strings.TrimSpace(cmd.IdempotencyKey),
		)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid Idempotency-Key: %w",
				err,
			)
		}

		applicationID = uuid.NewSHA1(
			uuid.NameSpaceOID,
			[]byte("loan:"+cmd.CustomerID+":"+key.String()),
		).String()

		previous, err := h.repo.FindByID(ctx, applicationID)
		if err != nil {
			return nil, err
		}

		if previous != nil {
			if previous.CustomerId != cmd.CustomerID ||
				previous.LoanPackageId != cmd.LoanPackageID ||
				previous.DisbursementAccountId != cmd.DisbursementAccountID ||
				previous.RequestedAmount != cmd.RequestedAmount ||
				previous.TermMonths != cmd.TermMonths {
				return nil, fmt.Errorf(
					"loan idempotency key reused with a different request",
				)
			}

			return previous, nil
		}
	}

	applicationNo := "LA-" + strings.ToUpper(applicationID)

	// 5. Sử dụng Domain để khởi tạo hồ sơ.
	app, err := loandomain.NewDraftApplication(
		applicationID,
		applicationNo,
		cmd.CustomerID,
		cmd.LoanPackageID,
		cmd.DisbursementAccountID,
		cmd.RequestedAmount,
		cmd.TermMonths,
	)
	if err != nil {
		return nil, err
	}

	// 6. Xác minh tài khoản nhận giải ngân.
	//
	// Implementation của validator phải kiểm tra:
	// - Tài khoản thuộc đúng khách hàng.
	// - Tài khoản còn hoạt động.
	// - Tài khoản đáp ứng chính sách nhận lương.
	//
	// Không được bỏ qua kiểm tra này khi tích hợp API.
	if err := h.validator.ValidateDisbursementAccount(
		ctx,
		app.CustomerId,
		app.DisbursementAccountId,
	); err != nil {
		return nil, fmt.Errorf(
			"invalid loan disbursement account: %w",
			err,
		)
	}

	// 7. Lưu hồ sơ vào EventStoreDB thông qua port.
	//
	// Repository.Create sẽ ghi:
	// LoanApplicationCreated.
	if err := h.repo.Create(ctx, app); err != nil {
		if cmd.IdempotencyKey != "" {
			previous, readErr := h.repo.FindByID(
				ctx,
				applicationID,
			)

			if readErr == nil &&
				previous != nil &&
				previous.CustomerId == cmd.CustomerID &&
				previous.LoanPackageId == cmd.LoanPackageID &&
				previous.DisbursementAccountId == cmd.DisbursementAccountID &&
				previous.RequestedAmount == cmd.RequestedAmount &&
				previous.TermMonths == cmd.TermMonths {
				return previous, nil
			}
		}
		return nil, fmt.Errorf(
			"create loan application failed: %w",
			err,
		)
	}

	// 8. Trả hồ sơ vừa tạo.
	return app, nil
}

// Những kỳ hạn hỗ trợ theo chính sách hiện tại.
func isSupportedLoanTerm(months int) bool {
	switch months {
	case 6, 12, 24, 36, 48, 60:
		return true
	default:
		return false
	}
}
