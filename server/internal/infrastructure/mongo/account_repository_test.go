package mongo

import (
	"errors"
	"fmt"
	"testing"

	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
)

func TestIsDuplicateKeyError(t *testing.T) {
	duplicateWriteErr := mongodrv.WriteException{
		WriteErrors: mongodrv.WriteErrors{{Code: 11000, Message: "duplicate key"}},
	}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil},
		{name: "unrelated", err: errors.New("mongo unavailable")},
		{name: "write exception", err: duplicateWriteErr, want: true},
		{name: "wrapped write exception", err: fmt.Errorf("save account: %w", duplicateWriteErr), want: true},
		{name: "command error", err: &mongodrv.CommandError{Code: 11000, Message: "duplicate key"}, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsDuplicateKeyError(test.err); got != test.want {
				t.Fatalf("IsDuplicateKeyError() = %v, want %v", got, test.want)
			}
		})
	}
}
