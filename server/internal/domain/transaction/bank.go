package transaction

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/timeutil"
)

const (
	BankStream               = "banking"
	AccountCreated           = "BankAccountCreated"
	AccountUpdated           = "BankAccountUpdated"
	BalanceSeeded            = "BankBalanceSeeded"
	BankImported             = "BankImported"
	LoanDisbursementCredited = "LoanDisbursementCredited"
	LoanInstallmentPaid      = "LoanInstallmentPaid"
)

// LoanInstallmentPayment là dữ liệu thanh toán một kỳ.
// Amount = PrincipalAmount + InterestAmount.
// MVP chưa áp dụng tiền phạt.
type LoanInstallmentPayment struct {
	PaymentID                 string    `json:"paymentId"`
	TransactionID             string    `json:"transactionId"`
	LoanID                    string    `json:"loanId"`
	CustomerID                string    `json:"customerId"`
	AccountID                 string    `json:"accountId"`
	InstallmentNo             int       `json:"installmentNo"`
	DueDate                   time.Time `json:"dueDate"`
	PaidAt                    time.Time `json:"paidAt"`
	PrincipalAmount           int64     `json:"principalAmount"`
	InterestAmount            int64     `json:"interestAmount"`
	Amount                    int64     `json:"amount"`
	OutstandingPrincipalAfter int64     `json:"outstandingPrincipalAfter"`
}

func LoanInstallmentTransactionID(
	loanID string,
	installmentNo int,
) string {
	key := fmt.Sprintf(
		"loan-installment:%s:%d",
		loanID,
		installmentNo,
	)
	return EventID(key, "transaction")
}

func (p LoanInstallmentPayment) Validate() error {
	if strings.TrimSpace(p.LoanID) == "" ||
		strings.TrimSpace(p.CustomerID) == "" ||
		strings.TrimSpace(p.AccountID) == "" ||
		p.InstallmentNo <= 0 ||
		p.PrincipalAmount <= 0 ||
		p.InterestAmount < 0 ||
		p.Amount <= 0 ||
		p.PrincipalAmount > p.Amount ||
		p.InterestAmount != p.Amount-p.PrincipalAmount ||
		p.OutstandingPrincipalAfter < 0 ||
		p.DueDate.IsZero() ||
		p.PaidAt.IsZero() {
		return fmt.Errorf("invalid loan installment payment")
	}

	expectedTx := LoanInstallmentTransactionID(
		p.LoanID,
		p.InstallmentNo,
	)

	if p.TransactionID != expectedTx ||
		p.PaymentID != EventID(expectedTx, "payment") {
		return fmt.Errorf("invalid loan payment identifiers")
	}

	// DueDate duoc luu theo ngay UTC trong lich EMI.
	// PaidAt duoc so sanh theo ngay Viet Nam.
	vn := timeutil.VietnamLocation

	y, m, d := p.PaidAt.In(vn).Date()
	dy, dm, dd := p.DueDate.UTC().Date()

	if y != dy || m != dm || d != dd {
		return fmt.Errorf("loan installment paid outside due date")
	}

	return nil
}

// Record is the canonical banking journal. Transfer snapshots retain the HTTP
// contract; downstream consumers can extract Event without profile credentials.
type Record struct {
	ID            string                       `json:"id"`
	Type          string                       `json:"type"`
	At            time.Time                    `json:"at"`
	Account       *accountdomain.UserAccount   `json:"account,omitempty"`
	Accounts      []*accountdomain.UserAccount `json:"accounts,omitempty"`
	AccountID     string                       `json:"accountId,omitempty"`
	Balance       *int64                       `json:"balance,omitempty"`
	Event         *Event                       `json:"event,omitempty"`
	Transfer      json.RawMessage              `json:"transfer,omitempty"`
	Historical    bool                         `json:"historical,omitempty"`
	LoanID        string                       `json:"loanId,omitempty"`
	LoanAmount    int64                        `json:"loanAmount,omitempty"`
	BalanceBefore *int64                       `json:"balanceBefore,omitempty"`
	BalanceAfter  *int64                       `json:"balanceAfter,omitempty"`
	LoanPayment   *LoanInstallmentPayment      `json:"loanPayment,omitempty"`
}

func DecodeRecord(data []byte, eventID, eventType string) (Record, error) {
	var r Record
	if err := json.Unmarshal(data, &r); err != nil {
		return r, err
	}
	if r.ID != eventID || r.Type != eventType || r.At.IsZero() {
		return r, fmt.Errorf("banking record metadata mismatch")
	}
	if IsTransferEvent(r.Type) {
		if r.Event == nil || r.Event.ID != r.ID || r.Event.Type != r.Type || len(r.Transfer) == 0 {
			return r, fmt.Errorf("missing transfer event/snapshot")
		}
		if err := r.Event.Validate(); err != nil {
			return r, err
		}
		return r, nil
	}
	switch r.Type {
	case AccountCreated, AccountUpdated:
		if r.Account == nil || r.Account.Customer == nil {
			return r, fmt.Errorf("missing account aggregate")
		}
	case BalanceSeeded:
		if r.AccountID == "" || r.Balance == nil || *r.Balance < 0 {
			return r, fmt.Errorf("invalid balance seed")
		}
	case BankImported:
		for _, a := range r.Accounts {
			if a == nil || a.Customer == nil || a.Balance.Current < 0 {
				return r, fmt.Errorf("invalid imported account")
			}
		}
	case LoanDisbursementCredited:
		if r.LoanID == "" ||
			r.AccountID == "" ||
			r.LoanAmount <= 0 ||
			r.BalanceBefore == nil ||
			r.BalanceAfter == nil ||
			*r.BalanceBefore < 0 ||
			r.LoanAmount > math.MaxInt64-*r.BalanceBefore ||
			*r.BalanceAfter != *r.BalanceBefore+r.LoanAmount ||
			r.ID != EventID(
				r.LoanID,
				LoanDisbursementCredited,
			) {
			return r, fmt.Errorf(
				"invalid loan credit journal",
			)
		}

	case LoanInstallmentPaid:
		if r.LoanPayment == nil {
			return r, fmt.Errorf("missing loan payment")
		}

		p := r.LoanPayment

		if err := p.Validate(); err != nil {
			return r, err
		}

		if r.ID != EventID(p.TransactionID, LoanInstallmentPaid) ||
			r.AccountID != p.AccountID ||
			r.LoanID != p.LoanID ||
			r.BalanceBefore == nil ||
			r.BalanceAfter == nil ||
			*r.BalanceBefore < p.Amount ||
			*r.BalanceAfter != *r.BalanceBefore-p.Amount ||
			!r.At.Equal(p.PaidAt) {
			return r, fmt.Errorf("invalid loan repayment journal")
		}
	default:
		return r, fmt.Errorf("unsupported banking record %q", r.Type)
	}
	return r, nil
}
