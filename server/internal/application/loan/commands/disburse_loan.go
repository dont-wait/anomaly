package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	domain "github.com/dont-wait/anomaly/internal/domain/loan"
)

type LoanEvidenceReader interface {
	Find(
		context.Context,
		string,
	) (domain.VerifiedEvidence, error)
}

type LoanCreditService interface {
	CreditLoanDisbursement(
		context.Context,
		string,
		string,
		int64,
	) (time.Time, error)
}

type DisburseLoanCommand struct {
	ApplicationID string
	ActorID       string
}

type DisburseLoanCommandHandler struct {
	Repo       LoanDocumentRepository
	Authorizer LoanManualDecisionAuthorizer
	Evidence   LoanEvidenceReader
	Bank       LoanCreditService
}

func (h *DisburseLoanCommandHandler) Handle(
	ctx context.Context,
	cmd DisburseLoanCommand,
) (*domain.Application, error) {
	if h == nil ||
		h.Repo == nil ||
		h.Authorizer == nil ||
		h.Evidence == nil ||
		h.Bank == nil {
		return nil, fmt.Errorf(
			"loan disbursement services unavailable",
		)
	}

	cmd.ApplicationID = strings.TrimSpace(cmd.ApplicationID)
	cmd.ActorID = strings.TrimSpace(cmd.ActorID)

	if cmd.ApplicationID == "" || cmd.ActorID == "" {
		return nil, fmt.Errorf("loan and actor required")
	}

	if err := h.Authorizer.AuthorizeLoanManualDecision(
		ctx,
		cmd.ActorID,
	); err != nil {
		return nil, err
	}

	a, revision, err := h.Repo.FindByIDWithRevision(
		ctx,
		cmd.ApplicationID,
	)
	if err != nil {
		return nil, err
	}

	if a == nil || a.Id != cmd.ApplicationID {
		return nil, fmt.Errorf("loan not found")
	}

	// Nếu đã giải ngân thì trả trạng thái cũ.
	if a.Status == domain.ApplicationStatusDisbursed {
		return a, nil
	}

	if a.Status != domain.ApplicationStatusApproved {
		return nil, domain.ErrInvalidApplicationStatus
	}

	// Đọc dữ liệu thẩm định được lưu trước đó.

	// Nếu đã chuẩn bị giải ngân, sử dụng lãi suất đã chốt.
	rate := a.PreparedAnnualRateBps

	if a.DisbursementPreparedAt == nil {
		// Chưa chuẩn bị: kiểm tra chứng cứ và tính lãi suất.
		evidence, err := h.Evidence.Find(ctx, a.Id)
		if err != nil {
			return nil, fmt.Errorf(
				"required loan evidence is unavailable: %w",
				err,
			)
		}

		if !evidence.FinancialDataVerified ||
			!evidence.DocumentsVerified ||
			!evidence.InsuranceVerified ||
			!evidence.HistoryVerified {
			return nil, fmt.Errorf(
				"loan evidence is not fully verified for disbursement",
			)
		}

		input, calculatedRate, err := evidence.BuildAssessment(
			a,
			time.Now().UTC(),
		)
		if err != nil {
			return nil, err
		}

		assessment, err := domain.AssessLoanApplication(input)
		if err != nil {
			return nil, err
		}

		if assessment.Decision != a.AssessmentDecision ||
			assessment.TotalScore != a.AssessmentScore {
			return nil, fmt.Errorf(
				"disbursement evidence does not match approved assessment",
			)
		}

		rate = calculatedRate
		preparedAt := time.Now().UTC()

		prepared := *a
		if err := prepared.PrepareDisbursement(
			preparedAt,
			rate,
		); err != nil {
			return nil, err
		}

		payload := struct {
			PreparedAt    time.Time `json:"preparedAt"`
			AnnualRateBps int64     `json:"annualRateBps"`
			UpdatedAt     time.Time `json:"updatedAt"`
		}{
			PreparedAt:    preparedAt,
			AnnualRateBps: rate,
			UpdatedAt:     preparedAt,
		}

		// Phải ghi thành công trước khi cộng tiền.
		if err := h.Repo.Append(
			ctx,
			a.Id,
			revision,
			"LoanDisbursementPrepared",
			payload,
		); err != nil {
			return nil, fmt.Errorf(
				"prepare loan disbursement: %w",
				err,
			)
		}

		a = &prepared
		revision++
	} else if rate <= 0 {
		return nil, fmt.Errorf(
			"invalid prepared loan interest rate",
		)
	}

	// Ghi tiền vào banking stream với LoanID
	// để tránh cộng tiền hai lần khi retry.
	paidAt, err := h.Bank.CreditLoanDisbursement(
		ctx,
		a.Id,
		a.DisbursementAccountId,
		a.RequestedAmount,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"bank loan credit failed: %w",
			err,
		)
	}

	preview := *a

	if err := preview.MarkDisbursed(
		paidAt,
		rate,
	); err != nil {
		return nil, err
	}

	payload := struct {
		DisbursedAt       time.Time                      `json:"disbursedAt"`
		AnnualRateBps     int64                          `json:"annualRateBps"`
		RepaymentSchedule []domain.RepaymentScheduleItem `json:"repaymentSchedule"`
		UpdatedAt         time.Time                      `json:"updatedAt"`
	}{
		DisbursedAt:       paidAt,
		AnnualRateBps:     rate,
		RepaymentSchedule: preview.RepaymentSchedule,
		UpdatedAt:         preview.UpdatedAt,
	}

	// Ghi LoanDisbursed vào EventStoreDB.
	if err := h.Repo.Append(
		ctx,
		a.Id,
		revision,
		"LoanDisbursed",
		payload,
	); err != nil {
		return nil, fmt.Errorf(
			"bank credited (loan %s); retry finalizing loan event: %w",
			a.Id,
			err,
		)
	}

	return &preview, nil
}
