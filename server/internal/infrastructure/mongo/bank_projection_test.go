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

func TestImportedBaselineAlreadyApplied(t *testing.T) {
	one, two := int64(1), int64(2)
	cases := []struct {
		name    string
		marker  *int64
		current int64
		want    bool
	}{
		{"not yet imported", nil, 7, false},
		{"first projection complete", &one, 1, true},
		{"replay after deposits", &one, 9, true},
		{"corrupt version", &one, 0, false},
		{"wrong import marker", &two, 9, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := importedBaselineAlreadyApplied(tc.marker, tc.current, 1); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
