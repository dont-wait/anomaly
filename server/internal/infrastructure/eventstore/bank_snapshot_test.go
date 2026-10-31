package eventstore

import (
	"testing"

	"github.com/dont-wait/anomaly/internal/domain/account"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
)

func TestBankIncrementalSnapshotDoesNotShareMutableMaps(t *testing.T) {
	original := &bankState{
		Exists: true, Revision: 123,
		Accounts:  map[string]*account.UserAccount{"actor": {Id: "actor"}},
		Balances:  map[string]int64{"account": 100},
		Transfers: map[string]transferState{}, Keys: map[string]string{},
		LoanCredits: map[string]loanCreditState{}, LoanPayments: map[string]journal.LoanInstallmentPayment{},
		Savings: map[string]journal.SavingsMovement{}, SavingsKeys: map[string]string{},
		SavingsQuotes: map[string]string{}, SavingsWithdrawalKeys: map[string]string{},
	}
	next := cloneBankState(original)
	next.Balances["account"] = 0
	next.SavingsKeys["key"] = "contract"
	next.Accounts["actor"] = &account.UserAccount{Id: "changed"}
	if original.Balances["account"] != 100 || original.SavingsKeys["key"] != "" || original.Accounts["actor"].Id != "actor" || next.Revision != original.Revision {
		t.Fatal("incremental bank snapshot mutated prior authoritative state")
	}
}
