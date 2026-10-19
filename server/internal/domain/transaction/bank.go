package transaction

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

const (
	BankStream               = "banking"
	AccountCreated           = "BankAccountCreated"
	AccountUpdated           = "BankAccountUpdated"
	BalanceSeeded            = "BankBalanceSeeded"
	BankImported             = "BankImported"
	LoanDisbursementCredited = "LoanDisbursementCredited"
)

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
	default:
		return r, fmt.Errorf("unsupported banking record %q", r.Type)
	}
	return r, nil
}
