package eventstore

import (
	"errors"
	"testing"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
)

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
