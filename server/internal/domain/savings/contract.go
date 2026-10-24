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
	// Revision is the authoritative EventStoreDB stream revision loaded by the
	// repository. Settlement must be claimed with a compare-and-swap against
	// this revision BEFORE any account credit is attempted.
	Revision uint64
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
	// GĐ1 opens contracts only at an approved term/rate combination.
	// Preserve that validated rate as a snapshot for all future payouts.
	// In GĐ2/GĐ3 the catalogue/quote authority must validate the offered
	// policy version before constructing the contract.
	approvedRate, err := AnnualRateBpsForTerm(p.TermMonths)
	if err != nil {
		return nil, err
	}
	if p.AnnualRateBps != approvedRate {
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
	if c.OpenedAt.IsZero() || c.MaturityAt.IsZero() || !c.MaturityAt.After(c.OpenedAt) {
		return ErrInvalidMaturity
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

// WithdrawalIntent describes the ONE permitted settlement of this contract.
// Two independently loaded copies MUST yield the same IdempotencyKey. The
// application layer must use it for the bank credit and atomically claim the
// expected EventStoreDB revision before issuing that credit; checking the
// in-memory Contract.Status alone cannot prevent concurrent double credits.
type WithdrawalIntent struct {
	ContractID       string
	IdempotencyKey   string
	ExpectedRevision uint64
	InterestAmount   int64
	TotalAmount      int64
}

func (c *Contract) PrepareWithdrawal(now time.Time) (WithdrawalIntent, error) {
	if err := c.CanWithdraw(now); err != nil {
		return WithdrawalIntent{}, err
	}
	if strings.TrimSpace(c.ID) == "" {
		return WithdrawalIntent{}, ErrInvalidContract
	}
	interest, total, err := CalculateMaturityPayout(c.Principal, c.AnnualRateBps, c.OpenedAt, c.MaturityAt)
	if err != nil {
		return WithdrawalIntent{}, err
	}
	return WithdrawalIntent{
		ContractID:       c.ID,
		IdempotencyKey:   "savings:settlement:" + c.ID,
		ExpectedRevision: c.Revision,
		InterestAmount:   interest,
		TotalAmount:      total,
	}, nil
}

// ConfirmSettled applies the durable settlement result to the in-memory
// contract AFTER the repository has atomically claimed ExpectedRevision and
// the banking layer has confirmed its single idempotent credit. This method
// does not perform CAS or move money; those controls MUST be implemented by
// the command/repository integration in GĐ3 and tested in GĐ5.
func (c *Contract) ConfirmSettled(intent WithdrawalIntent, transactionID string, committedRevision uint64, now time.Time) error {
	if err := c.CanWithdraw(now); err != nil {
		return err
	}
	if strings.TrimSpace(transactionID) == "" {
		return ErrInvalidSettlement
	}
	current, err := c.PrepareWithdrawal(now)
	if err != nil {
		return err
	}
	if intent != current || committedRevision <= intent.ExpectedRevision || committedRevision != intent.ExpectedRevision+1 {
		return ErrSettlementConflict
	}
	c.Settlement = &Settlement{
		TransactionID: transactionID, InterestAmount: current.InterestAmount,
		TotalAmount: current.TotalAmount, SettledAt: now,
	}
	c.Status = ContractSettled
	c.Revision = committedRevision
	return nil
}
