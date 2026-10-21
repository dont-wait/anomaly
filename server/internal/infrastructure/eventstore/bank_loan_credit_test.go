package eventstore

import (
	"testing"
	"time"

	acct "github.com/dont-wait/anomaly/internal/domain/account"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
)

func TestBankLoanCreditDoesNotAllowDuplicateLoanID(t *testing.T) {
	s := bankState{
		Accounts: map[string]*acct.UserAccount{
			"owner": {
				Id: "owner",
			},
		},
		Balances: map[string]int64{
			financialID("owner").Hex(): 100,
		},
		LoanCredits: map[string]loanCreditState{},
	}

	before := int64(100)
	after := int64(200)

	rec := journal.Record{
		ID: journal.EventID(
			"loan-1",
			journal.LoanDisbursementCredited,
		),
		Type:          journal.LoanDisbursementCredited,
		At:            time.Now().UTC(),
		AccountID:     "owner",
		LoanID:        "loan-1",
		LoanAmount:    100,
		BalanceBefore: &before,
		BalanceAfter:  &after,
	}

	if err := s.apply(rec); err != nil {
		t.Fatal(err)
	}

	if s.Balances[financialID("owner").Hex()] != 200 {
		t.Fatal("balance credit missing")
	}

	// Không được ghi cùng LoanID lần hai.
	if err := s.apply(rec); err == nil {
		t.Fatal("double disbursement event accepted")
	}

	if s.Balances[financialID("owner").Hex()] != 200 {
		t.Fatal("duplicate credit changed balance")
	}
}
