package eventstore

import (
	"testing"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
)

func TestBankLoanRepaymentReplayAndDuplicate(t *testing.T) {
	now := time.Now().UTC()
	vn := time.FixedZone("ICT", 7*3600)
	y, m, d := now.In(vn).Date()

	const (
		accountID = "owner-001"
		loanID    = "loan-001"
	)

	txID := journal.LoanInstallmentTransactionID(
		loanID,
		1,
	)

	payment := journal.LoanInstallmentPayment{
		PaymentID:     journal.EventID(txID, "payment"),
		TransactionID: txID,
		LoanID:        loanID,
		CustomerID:    accountID,
		AccountID:     accountID,
		InstallmentNo: 1,
		DueDate: time.Date(
			y, m, d, 0, 0, 0, 0, time.UTC,
		),
		PaidAt:                    now,
		PrincipalAmount:           900_000,
		InterestAmount:            100_000,
		Amount:                    1_000_000,
		OutstandingPrincipalAfter: 9_100_000,
	}

	state := &bankState{
		Accounts: map[string]*accountdomain.UserAccount{
			accountID: {
				Id: accountID,
			},
		},
		Balances: map[string]int64{
			financialID(accountID).Hex(): 2_000_000,
		},
		LoanCredits: map[string]loanCreditState{
			loanID: {
				AccountID: accountID,
				Amount:    10_000_000,
				At:        now.AddDate(0, -1, 0),
			},
		},
		LoanPayments: map[string]journal.LoanInstallmentPayment{},
	}

	before := int64(2_000_000)
	after := int64(1_000_000)

	record := journal.Record{
		ID: journal.EventID(
			txID,
			journal.LoanInstallmentPaid,
		),
		Type:          journal.LoanInstallmentPaid,
		At:            now,
		AccountID:     accountID,
		LoanID:        loanID,
		BalanceBefore: &before,
		BalanceAfter:  &after,
		LoanPayment:   &payment,
	}

	if err := state.apply(record); err != nil {
		t.Fatal(err)
	}

	if state.Balances[financialID(accountID).Hex()] != after {
		t.Fatal("loan debit balance mismatch")
	}

	if len(state.LoanPayments) != 1 {
		t.Fatal("loan payment not recorded")
	}

	// Replay them mot event trung phai bi tu choi.
	if err := state.apply(record); err == nil {
		t.Fatal("duplicate loan payment accepted")
	}

	// So du khong duoc thay doi lan hai.
	if state.Balances[financialID(accountID).Hex()] != after {
		t.Fatal("duplicate loan payment changed balance")
	}
}
