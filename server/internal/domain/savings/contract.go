package savings

import (
	"strings"
	"time"
)

type ContractStatus string

const (
	ContractActive  ContractStatus = "active"
	ContractMatured ContractStatus = "matured"
	ContractSettled ContractStatus = "settled"
	ContractLocked  ContractStatus = "locked"
)

type Settlement struct {
	TransactionID  string
	InterestAmount int64
	TotalAmount    int64
	SettledAt      time.Time
}

// Contract is a savings read/write domain model, not a MongoDB record.
// EventStoreDB persistence and bank-account debit/credit belong to GĐ3–GĐ4.
// The annual rate is a snapshot of the rate chosen at opening, not a lookup
// against the latest product policy when the customer later withdraws.
type Contract struct {
	ID              string
	SavingsNo       string
	CustomerID      string
	PackageID       string
	SourceAccountID string
	Principal       int64
	TermMonths      int
	AnnualRateBps   int64
	OpenedAt        time.Time
	MaturityAt      time.Time
	Status          ContractStatus
	Settlement      *Settlement
}

type OpenContractParams struct {
	ID              string
	SavingsNo       string
	CustomerID      string
	PackageID       string
	SourceAccountID string
	Principal       int64
	TermMonths      int
	AnnualRateBps   int64
	OpenedAt        time.Time
}

// NewContract validates domain data but DOES NOT debit funds or persist it.
// A contract must only become visible as active once GĐ3's transactional
// banking/event-sourcing flow has safely recorded the transfer.
func NewContract(p OpenContractParams) (*Contract, error) {
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.SavingsNo) == "" ||
		strings.TrimSpace(p.CustomerID) == "" || strings.TrimSpace(p.PackageID) == "" ||
		strings.TrimSpace(p.SourceAccountID) == "" {
		return nil, ErrInvalidContract
	}
	if err := ValidateDeposit(p.Principal, p.TermMonths); err != nil {
		return nil, err
	}
	// Current policy lives in the packages catalog. Accept an already
	// confirmed rate snapshot here so a future policy update cannot
	// silently change an existing savings contract.
	if p.AnnualRateBps <= 0 || p.AnnualRateBps > 10_000 {
		return nil, ErrInvalidInterestRate
	}
	maturity, err := CalculateMaturityAt(p.OpenedAt, p.TermMonths)
	if err != nil {
		return nil, err
	}
	if _, _, err := CalculateMaturityPayout(p.Principal, p.AnnualRateBps, p.OpenedAt, maturity); err != nil {
		return nil, err
	}
	return &Contract{
		ID: p.ID, SavingsNo: p.SavingsNo, CustomerID: p.CustomerID,
		PackageID: p.PackageID, SourceAccountID: p.SourceAccountID,
		Principal: p.Principal, TermMonths: p.TermMonths,
		AnnualRateBps: p.AnnualRateBps, OpenedAt: p.OpenedAt,
		MaturityAt: maturity, Status: ContractActive,
	}, nil
}

// CanWithdraw determines eligibility from maturity time itself. It must not
// wait for the once-per-minute notification scheduler or MongoDB projection.
func (c *Contract) CanWithdraw(now time.Time) error {
	if c == nil {
		return ErrInvalidContract
	}
	if c.Status == ContractSettled {
		return ErrAlreadySettled
	}
	if c.Status == ContractLocked {
		return ErrContractLocked
	}
	if c.Status != ContractActive && c.Status != ContractMatured {
		return ErrInvalidContract
	}
	if now.IsZero() || now.Before(c.MaturityAt) {
		return ErrNotMatured
	}
	return nil
}

// MarkMatured is a non-monetary state transition. The scheduler will also
// create one in-app notification later; it must never credit the account.
func (c *Contract) MarkMatured(now time.Time) error {
	if err := c.CanWithdraw(now); err != nil {
		return err
	}
	c.Status = ContractMatured
	return nil
}

// ConfirmSettled can only be called by the application layer AFTER it has
// durably confirmed a single bank-account credit through EventStoreDB.
// This method itself never changes a payment account's balance.
func (c *Contract) ConfirmSettled(transactionID string, now time.Time) error {
	if err := c.CanWithdraw(now); err != nil {
		return err
	}
	if strings.TrimSpace(transactionID) == "" {
		return ErrInvalidSettlement
	}
	interest, total, err := CalculateMaturityPayout(c.Principal, c.AnnualRateBps, c.OpenedAt, c.MaturityAt)
	if err != nil {
		return err
	}
	c.Settlement = &Settlement{TransactionID: transactionID, InterestAmount: interest,
		TotalAmount: total, SettledAt: now}
	c.Status = ContractSettled
	return nil
}
