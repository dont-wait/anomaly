package eventstore

import (
	"context"
	"errors"
	"testing"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
)

// The API composition passes BankRepository, not the Mongo account repository.
// Keep its optional admin lookup contract checked at compile time.
var _ interface {
	FindByFinancialID(context.Context, string) (*accountdomain.UserAccount, error)
	FindByAccountNo(context.Context, string) (*accountdomain.UserAccount, error)
} = (*BankRepository)(nil)

func TestAccountByFinancialIDResolvesPublicAccountID(t *testing.T) {
	const accountID = "697dcd791d549fbe9a2a9423c6455d7d"
	fid := financialID(accountID).Hex()
	state := bankState{
		Accounts: map[string]*accountdomain.UserAccount{
			accountID: {Id: accountID, AccountNo: "79548859179658", Status: accountdomain.AccountStatusActive},
		},
		Balances: map[string]int64{fid: 123000},
	}
	got, err := state.accountByFinancialID(fid)
	if err != nil || got == nil {
		t.Fatalf("financial lookup = (%v, %v), want account", got, err)
	}
	if got.Id != accountID || got.Balance.Current != 123000 {
		t.Fatalf("resolved account = %#v, want public ID and ledger balance", got)
	}
	got.Status = accountdomain.AccountStatusLocked
	if state.Accounts[accountID].Status != accountdomain.AccountStatusActive {
		t.Fatal("lookup must return a copy, not mutate replay state")
	}
	missing, err := state.accountByFinancialID("000000000000000000000000")
	if err != nil || missing != nil {
		t.Fatalf("missing lookup = (%v, %v), want nil", missing, err)
	}
}

func TestTransferByIdempotencyKeyIsOwnerScopedAndCaseInsensitive(t *testing.T) {
	const (
		owner      = "owner-id"
		otherOwner = "other-owner-id"
		key        = "34297a72-9433-4adf-8958-868d5e50d197"
	)
	tx := mongorepo.TransferDocument{ID: "transfer-id", IdempotencyKey: key}
	tx.Source.AccountID = financialID(owner)
	state := bankState{
		Accounts: map[string]*accountdomain.UserAccount{
			owner:      {Id: owner},
			otherOwner: {Id: otherOwner},
		},
		Transfers: map[string]transferState{tx.ID: {Tx: tx}},
		Keys:      map[string]string{key: tx.ID},
	}

	got, err := state.transferByIdempotencyKey("34297A72-9433-4ADF-8958-868D5E50D197", owner)
	if err != nil || got.ID != tx.ID {
		t.Fatalf("owner lookup = (%q, %v), want transfer %q", got.ID, err, tx.ID)
	}
	if _, err := state.transferByIdempotencyKey(key, otherOwner); !errors.Is(err, mongorepo.ErrTransactionAccountNotFound) {
		t.Fatalf("other owner error = %v, want account not found", err)
	}
}

func TestSeedBalanceByAccountNoBuildsIncrementalBalanceEvent(t *testing.T) {
	const accountID = "account-id"
	state := bankState{
		Accounts: map[string]*accountdomain.UserAccount{
			accountID: {
				Id:        accountID,
				AccountNo: "12345678901234",
				Status:    accountdomain.AccountStatusActive,
			},
		},
		Balances: map[string]int64{financialID(accountID).Hex(): 125000},
	}

	record, err := state.seedBalanceByAccountNo("12345678901234", 75000, "operation-id")
	if err != nil {
		t.Fatal(err)
	}
	if record.Type != journal.BalanceSeeded {
		t.Fatalf("record type = %q, want %q", record.Type, journal.BalanceSeeded)
	}
	if record.AccountID != accountID || record.Balance == nil || *record.Balance != 200000 {
		t.Fatalf("record = %#v, want account %q and balance 200000", record, accountID)
	}
}

func TestSeedBalanceByAccountNoRejectsUnknownOrInvalidInput(t *testing.T) {
	state := bankState{Accounts: map[string]*accountdomain.UserAccount{}, Balances: map[string]int64{}}
	if _, err := state.seedBalanceByAccountNo("missing", 1, "operation-id"); !errors.Is(err, accountdomain.ErrAccountNotFound) {
		t.Fatalf("missing account error = %v, want account not found", err)
	}
	if _, err := state.seedBalanceByAccountNo("", 1, "operation-id"); !errors.Is(err, accountdomain.ErrInvalidAmount) {
		t.Fatalf("empty account error = %v, want invalid amount", err)
	}
	if _, err := state.seedBalanceByAccountNo("missing", 0, "operation-id"); !errors.Is(err, accountdomain.ErrInvalidAmount) {
		t.Fatalf("zero amount error = %v, want invalid amount", err)
	}
}
