package mongo

import (
	"testing"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

func TestBankImportBalanceAt(t *testing.T) {
	updatedAt := time.Date(2026, time.March, 3, 4, 5, 6, 0, time.UTC)
	createdAt := time.Date(2025, time.February, 2, 3, 4, 5, 0, time.UTC)
	recordAt := time.Date(2024, time.January, 1, 2, 3, 4, 0, time.UTC)

	tests := []struct {
		name    string
		account *accountdomain.UserAccount
		want    time.Time
	}{
		{name: "updated at", account: &accountdomain.UserAccount{UpdatedAt: updatedAt, CreatedAt: createdAt}, want: updatedAt},
		{name: "created at fallback", account: &accountdomain.UserAccount{CreatedAt: createdAt}, want: createdAt},
		{name: "record at fallback", account: &accountdomain.UserAccount{}, want: recordAt},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := bankImportBalanceAt(test.account, recordAt); !got.Equal(test.want) {
				t.Fatalf("bankImportBalanceAt() = %v, want %v", got, test.want)
			}
		})
	}
}
