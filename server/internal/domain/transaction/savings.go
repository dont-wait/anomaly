package transaction

import (
	"fmt"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/savings"
)

const (
	SavingsOpened  = "SavingsOpened"
	SavingsSettled = "SavingsSettled"
)

type SavingsOpenInput struct {
	ActorID, QuoteID, PackageID, SourceAccountID, CustomerID, IdempotencyKey string
	Principal                                                                int64
	TermMonths                                                               int
	AnnualRateBps                                                            int64
	ExpiresAt                                                                time.Time
}

// SavingsMovement is the canonical contract AND money movement in a single
// banking-stream event. There is no intermediate debit-only/credit-only state.
type SavingsMovement struct {
	Contract          savings.Contract `json:"contract"`
	ActorID           string           `json:"actorId"`
	OpeningQuoteID    string           `json:"openingQuoteId"`
	OpeningKey        string           `json:"openingKey"`
	WithdrawalQuoteID string           `json:"withdrawalQuoteId,omitempty"`
	WithdrawalKey     string           `json:"withdrawalKey,omitempty"`
	TransactionID     string           `json:"transactionId"`
}

func (m SavingsMovement) Validate(eventType string) error {
	c := m.Contract
	if strings.TrimSpace(c.ID) == "" || strings.TrimSpace(m.ActorID) == "" ||
		strings.TrimSpace(m.OpeningQuoteID) == "" || strings.TrimSpace(m.OpeningKey) == "" ||
		strings.TrimSpace(m.TransactionID) == "" || c.CustomerID == "" || c.SourceAccountID == "" ||
		c.PackageID == "" || c.Principal < savings.MinimumDepositVND || c.OpenedAt.IsZero() ||
		c.MaturityAt.IsZero() || c.SavingsNo == "" ||
		!c.MaturityAt.After(c.OpenedAt) {
		return fmt.Errorf("invalid savings journal metadata")
	}
	// Replay uses the contract's rate snapshot; today's catalogue may have
	// changed since opening. Opening approval is enforced by NewContract.
	if c.AnnualRateBps <= 0 || c.AnnualRateBps > 10_000 {
		return fmt.Errorf("invalid savings contract rate")
	}
	maturity, err := savings.MaturityForStoredContract(c.OpenedAt, c.TermMonths)
	if err != nil || !maturity.Equal(c.MaturityAt) {
		return fmt.Errorf("invalid savings contract maturity")
	}
	switch eventType {
	case SavingsOpened:
		if c.Status != savings.ContractActive || c.Settlement != nil || m.WithdrawalKey != "" || m.WithdrawalQuoteID != "" || m.TransactionID != EventID(c.ID, "deposit") {
			return fmt.Errorf("invalid savings opening")
		}
	case SavingsSettled:
		if c.Status != savings.ContractSettled || c.Settlement == nil || m.WithdrawalKey == "" || m.WithdrawalQuoteID == "" || m.TransactionID != EventID(c.ID, "withdrawal") || c.Settlement.TransactionID != m.TransactionID || c.Settlement.SettledAt.Before(c.MaturityAt) {
			return fmt.Errorf("invalid savings settlement")
		}
		interest, total, err := savings.CalculateMaturityPayout(c.Principal, c.AnnualRateBps, c.OpenedAt, c.MaturityAt)
		if err != nil || c.Settlement.InterestAmount != interest || c.Settlement.TotalAmount != total {
			return fmt.Errorf("invalid savings settlement payout")
		}
	default:
		return fmt.Errorf("unknown savings movement")
	}
	return nil
}
