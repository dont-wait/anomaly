package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	domain "github.com/dont-wait/anomaly/internal/domain/loan"
)

// ReconcileCreditedLoanCommand chứa thông tin
// giải ngân đã được ghi trong Banking Stream.
type ReconcileCreditedLoanCommand struct {
	LoanID     string
	AccountID  string
	Amount     int64
	CreditedAt time.Time
}

type ReconcileCreditedLoanCommandHandler struct {
	Repo     LoanDocumentRepository
	Evidence LoanEvidenceReader
}

func (h *ReconcileCreditedLoanCommandHandler) Handle(
	ctx context.Context,
	cmd ReconcileCreditedLoanCommand,
) error {
	if h == nil || h.Repo == nil || h.Evidence == nil {
		return fmt.Errorf("loan reconciliation services unavailable")
	}

	cmd.LoanID = strings.TrimSpace(cmd.LoanID)
	cmd.AccountID = strings.TrimSpace(cmd.AccountID)

	if cmd.LoanID == "" ||
		cmd.AccountID == "" ||
		cmd.Amount <= 0 ||
		cmd.CreditedAt.IsZero() {
		return fmt.Errorf("invalid credited loan")
	}

	a, revision, err := h.Repo.FindByIDWithRevision(
		ctx,
		cmd.LoanID,
	)
	if err != nil {
		return err
	}

	if a == nil {
		return fmt.Errorf("credited loan application not found")
	}

	// Kiểm tra khoản tiền đã cộng đúng với hồ sơ.
	if a.DisbursementAccountId != cmd.AccountID ||
		a.RequestedAmount != cmd.Amount {
		return fmt.Errorf("credited loan does not match application")
	}

	// Đã hoàn tất trước đó thì không ghi thêm.
	if a.Status == domain.ApplicationStatusDisbursed {
		return nil
	}

	if a.Status != domain.ApplicationStatusApproved {
		return domain.ErrInvalidApplicationStatus
	}

	// Đọc lại chứng cứ thẩm định đã lưu.

	// Ưu tiên lãi suất đã chốt trước khi cộng tiền.
	rate := a.PreparedAnnualRateBps

	if a.DisbursementPreparedAt != nil {
		if rate <= 0 {
			return fmt.Errorf("invalid prepared loan interest rate")
		}
	} else {
		// Tương thích với các khoản đã được cộng tiền
		// trước khi có LoanDisbursementPrepared.
		evidence, err := h.Evidence.Find(ctx, a.Id)
		if err != nil {
			return fmt.Errorf("load loan evidence: %w", err)
		}

		if !evidence.FinancialDataVerified ||
			!evidence.DocumentsVerified ||
			!evidence.InsuranceVerified ||
			!evidence.HistoryVerified {
			return fmt.Errorf("loan evidence is not fully verified")
		}

		input, calculatedRate, err := evidence.BuildAssessment(
			a,
			cmd.CreditedAt,
		)
		if err != nil {
			return err
		}

		assessment, err := domain.AssessLoanApplication(input)
		if err != nil {
			return err
		}

		if assessment.Decision != a.AssessmentDecision ||
			assessment.TotalScore != a.AssessmentScore {
			return fmt.Errorf(
				"reconciliation assessment does not match approval",
			)
		}

		rate = calculatedRate
	}

	preview := *a

	if err := preview.MarkDisbursed(
		cmd.CreditedAt,
		rate,
	); err != nil {
		return err
	}

	payload := struct {
		DisbursedAt       time.Time                      `json:"disbursedAt"`
		AnnualRateBps     int64                          `json:"annualRateBps"`
		RepaymentSchedule []domain.RepaymentScheduleItem `json:"repaymentSchedule"`
		UpdatedAt         time.Time                      `json:"updatedAt"`
	}{
		DisbursedAt:       cmd.CreditedAt,
		AnnualRateBps:     rate,
		RepaymentSchedule: preview.RepaymentSchedule,
		UpdatedAt:         preview.UpdatedAt,
	}

	// Chỉ ghi LoanDisbursed.
	// KHÔNG gọi CreditLoanDisbursement lần nữa.
	if err := h.Repo.Append(
		ctx,
		a.Id,
		revision,
		"LoanDisbursed",
		payload,
	); err != nil {
		// Có thể API đã hoàn tất song song.
		// Đọc lại để kiểm tra trạng thái thực tế.
		latest, _, readErr := h.Repo.FindByIDWithRevision(
			ctx,
			a.Id,
		)

		if readErr == nil &&
			latest != nil &&
			latest.Status == domain.ApplicationStatusDisbursed &&
			latest.DisbursementAccountId == cmd.AccountID &&
			latest.RequestedAmount == cmd.Amount {
			return nil
		}

		if readErr != nil {
			return fmt.Errorf(
				"finalize loan failed: %v; reread failed: %w",
				err,
				readErr,
			)
		}

		return fmt.Errorf("finalize credited loan: %w", err)
	}

	return nil
}
