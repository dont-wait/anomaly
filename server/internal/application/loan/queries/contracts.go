package queries

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidLoanContractQuery = errors.New("invalid loan contract query")
	ErrLoanContractNotFound     = errors.New("loan contract not found")
	ErrLoanContractNotReady     = errors.New("loan contract projection is not ready")
)

// ContractSummary is the customer-facing view of a disbursed loan.
// The rate and principal come from the signed contract projection, not a
// recalculation using the current interest policy.
type ContractSummary struct {
	LoanID               string    `json:"loanId"`
	LoanNo               string    `json:"loanNo"`
	ApplicationID        string    `json:"applicationId"`
	LoanPackageID        string    `json:"loanPackageId"`
	Principal            int64     `json:"principal"`
	OutstandingPrincipal int64     `json:"outstandingPrincipal"`
	AnnualInterestRate   string    `json:"annualInterestRate"`
	TermMonths           int       `json:"termMonths"`
	Status               string    `json:"status"`
	DisbursedAt          time.Time `json:"disbursedAt"`
	MaturityDate         time.Time `json:"maturityDate"`
	PaidInstallments     int       `json:"paidInstallments"`
	TotalInstallments    int       `json:"totalInstallments"`
	NextInstallmentNo    int       `json:"nextInstallmentNo"`
	NextDueDate          string    `json:"nextDueDate,omitempty"`
	NextDueAmount        int64     `json:"nextDueAmount"`
}

type ContractDetail struct {
	LoanID               string        `json:"loanId"`
	LoanNo               string        `json:"loanNo"`
	ApplicationID        string        `json:"applicationId"`
	LoanPackageID        string        `json:"loanPackageId"`
	Principal            int64         `json:"principal"`
	OutstandingPrincipal int64         `json:"outstandingPrincipal"`
	AnnualInterestRate   string        `json:"annualInterestRate"`
	TermMonths           int           `json:"termMonths"`
	Status               string        `json:"status"`
	DisbursedAt          time.Time     `json:"disbursedAt"`
	MaturityDate         time.Time     `json:"maturityDate"`
	PaidInstallments     int           `json:"paidInstallments"`
	TotalInstallments    int           `json:"totalInstallments"`
	NextInstallmentNo    int           `json:"nextInstallmentNo"`
	NextDueDate          string        `json:"nextDueDate,omitempty"`
	NextDueAmount        int64         `json:"nextDueAmount"`
	RepaymentMethod      string        `json:"repaymentMethod"`
	Schedule             []Installment `json:"repaymentSchedule"`
}

type ContractListRow struct {
	Contract             ContractSummary
	ApplicationCreatedAt time.Time
	ApplicationID        string
}

type ContractListPage struct {
	Items               []ContractSummary `json:"items"`
	NextCursor          *string           `json:"nextCursor"`
	EventualConsistency bool              `json:"eventualConsistency"`
}

type LoanContractReader interface {
	ListContractPage(ctx context.Context, customerID string, limit int, afterTime time.Time, afterID string) ([]ContractListRow, bool, error)
	ReadContract(ctx context.Context, loanID, customerID string) (ContractDetail, error)
}

type LoanContractQueryService struct {
	Reader LoanContractReader
}

func NewLoanContractQueryService(reader LoanContractReader) *LoanContractQueryService {
	return &LoanContractQueryService{Reader: reader}
}

type contractCursor struct {
	CreatedAt time.Time `json:"createdAt"`
	ID        string    `json:"id"`
	Owner     string    `json:"owner"`
}

func (s *LoanContractQueryService) List(ctx context.Context, customerID string, limit int, token string) (ContractListPage, error) {
	if s == nil || s.Reader == nil || strings.TrimSpace(customerID) == "" || limit < 1 || limit > 50 || len(token) > 2048 {
		return ContractListPage{}, ErrInvalidLoanContractQuery
	}

	var after contractCursor
	if token != "" {
		data, err := base64.RawURLEncoding.DecodeString(token)
		if err != nil || json.Unmarshal(data, &after) != nil || after.CreatedAt.IsZero() || after.Owner != customerID {
			return ContractListPage{}, ErrInvalidLoanContractQuery
		}
		if _, err := uuid.Parse(after.ID); err != nil {
			return ContractListPage{}, ErrInvalidLoanContractQuery
		}
	}

	rows, hasMore, err := s.Reader.ListContractPage(ctx, customerID, limit, after.CreatedAt, after.ID)
	if err != nil {
		return ContractListPage{}, err
	}
	if len(rows) > limit {
		return ContractListPage{}, ErrLoanContractNotReady
	}

	page := ContractListPage{Items: make([]ContractSummary, 0, len(rows)), EventualConsistency: true}
	for _, row := range rows {
		page.Items = append(page.Items, row.Contract)
	}
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		encoded, err := json.Marshal(contractCursor{
			CreatedAt: last.ApplicationCreatedAt,
			ID:        last.ApplicationID,
			Owner:     customerID,
		})
		if err != nil {
			return ContractListPage{}, err
		}
		cursor := base64.RawURLEncoding.EncodeToString(encoded)
		page.NextCursor = &cursor
	}
	return page, nil
}

func (s *LoanContractQueryService) Get(ctx context.Context, loanID, customerID string) (ContractDetail, error) {
	if s == nil || s.Reader == nil || strings.TrimSpace(customerID) == "" {
		return ContractDetail{}, ErrInvalidLoanContractQuery
	}
	if _, err := uuid.Parse(loanID); err != nil {
		return ContractDetail{}, ErrInvalidLoanContractQuery
	}
	return s.Reader.ReadContract(ctx, loanID, customerID)
}
