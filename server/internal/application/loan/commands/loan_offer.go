package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	loan "github.com/dont-wait/anomaly/internal/domain/loan"
)

type LoanOfferCommandHandler struct {
	Repo       LoanDocumentRepository
	Authorizer LoanManualDecisionAuthorizer
	Evidence   LoanEvidenceReader
}

// Issue is a privileged operation using verified evidence; no client-supplied terms.
func (h *LoanOfferCommandHandler) Issue(ctx context.Context, applicationID, actorID string) (*loan.Application, error) {
	if h == nil || h.Repo == nil || h.Authorizer == nil || h.Evidence == nil {
		return nil, fmt.Errorf("loan offer services unavailable")
	}
	if err := h.Authorizer.AuthorizeLoanManualDecision(ctx, strings.TrimSpace(actorID)); err != nil {
		return nil, err
	}
	a, rev, err := h.Repo.FindByIDWithRevision(ctx, applicationID)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, fmt.Errorf("loan application not found")
	}
	if a.Offer != nil {
		return a, nil
	} // Idempotent only for previously issued offers, not new terms.
	if a.Status != loan.ApplicationStatusApproved || a.ApprovedAt == nil || a.DisbursementPreparedAt != nil {
		return nil, loan.ErrInvalidApplicationStatus
	}
	e, err := h.Evidence.Find(ctx, a.Id)
	if err != nil {
		return nil, fmt.Errorf("loan offer evidence unavailable: %w", err)
	}
	if !e.FinancialDataVerified || !e.DocumentsVerified || !e.InsuranceVerified || !e.HistoryVerified {
		return nil, fmt.Errorf("loan evidence is not fully verified")
	}
	input, rate, err := e.BuildAssessment(a, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	result, err := loan.AssessLoanApplication(input)
	if err != nil {
		return nil, err
	}
	if result.Decision != a.AssessmentDecision || result.TotalScore != a.AssessmentScore {
		return nil, fmt.Errorf("offer evidence does not match approved assessment")
	}
	at := time.Now().UTC()
	preview := *a
	if err := preview.IssueOffer(at, rate); err != nil {
		return nil, err
	}
	payload := struct {
		IssuedAt      time.Time `json:"issuedAt"`
		AnnualRateBps int64     `json:"annualRateBps"`
		UpdatedAt     time.Time `json:"updatedAt"`
	}{at, rate, at}
	if err := h.Repo.Append(ctx, a.Id, rev, "LoanOfferIssued", payload); err != nil {
		return nil, err
	}
	return &preview, nil
}

func (h *LoanOfferCommandHandler) transition(ctx context.Context, id, owner string, kind string) (*loan.Application, error) {
	if h == nil || h.Repo == nil {
		return nil, fmt.Errorf("loan offer services unavailable")
	}
	a, rev, err := h.Repo.FindByIDWithRevision(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil || a.CustomerId != owner {
		return nil, fmt.Errorf("loan application not found")
	}
	preview := *a
	if a.Offer != nil {
		offer := *a.Offer
		preview.Offer = &offer
	}
	at := time.Now().UTC()
	var event string
	switch kind {
	case "accept":
		if err := preview.AcceptOffer(at); err != nil {
			return nil, err
		}
		event = "LoanOfferAccepted"
	case "reject":
		if err := preview.RejectOffer(at); err != nil {
			return nil, err
		}
		event = "LoanOfferRejected"
	case "confirm":
		if err := preview.ConfirmContract(at, "email_otp"); err != nil {
			return nil, err
		}
		event = "LoanContractConfirmed"
	default:
		return nil, fmt.Errorf("invalid loan offer transition")
	}
	payload := struct {
		At        time.Time `json:"at"`
		UpdatedAt time.Time `json:"updatedAt"`
		Method    string    `json:"method,omitempty"`
	}{at, at, ""}
	if kind == "confirm" {
		payload.Method = "email_otp"
	}
	if err := h.Repo.Append(ctx, id, rev, event, payload); err != nil {
		return nil, err
	}
	return &preview, nil
}

func (h *LoanOfferCommandHandler) Accept(ctx context.Context, id, owner string) (*loan.Application, error) {
	return h.transition(ctx, id, owner, "accept")
}

func (h *LoanOfferCommandHandler) Reject(ctx context.Context, id, owner string) (*loan.Application, error) {
	return h.transition(ctx, id, owner, "reject")
}

func (h *LoanOfferCommandHandler) Confirm(ctx context.Context, id, owner string) (*loan.Application, error) {
	return h.transition(ctx, id, owner, "confirm")
}
