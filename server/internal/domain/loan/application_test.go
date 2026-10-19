package loan

import (
	"errors"
	"testing"
	"time"
)

// Kiểm tra tạo hồ sơ vay thành công.
func TestNewDraftApplication_Success(t *testing.T) {
	before := time.Now().UTC()

	app, err := NewDraftApplication(
		"app_001",
		"LA20261014001",
		"customer_001",
		"package_001",
		"account_001",
		50000000,
		12,
	)

	after := time.Now().UTC()

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if app == nil {
		t.Fatal("expected application, got nil")
	}

	if app.Status != ApplicationStatusDraft {
		t.Errorf("expected draft status, got %s", app.Status)
	}

	if app.RequestedAmount != 50000000 {
		t.Errorf("unexpected requested amount: %d", app.RequestedAmount)
	}

	if app.TermMonths != 12 {
		t.Errorf("expected 12 months, got %d", app.TermMonths)
	}

	if app.CustomerId != "customer_001" {
		t.Errorf("unexpected customer ID: %s", app.CustomerId)
	}

	if app.DisbursementAccountId != "account_001" {
		t.Errorf("unexpected disbursement account")
	}

	if app.CreatedAt.Before(before) || app.CreatedAt.After(after) {
		t.Error("createdAt is outside expected time range")
	}

	if !app.CreatedAt.Equal(app.UpdatedAt) {
		t.Error("createdAt and updatedAt must be equal")
	}
}

// Kiểm tra các dữ liệu đầu vào không hợp lệ.
func TestNewDraftApplication_InvalidInput(t *testing.T) {
	type input struct {
		id         string
		number     string
		customer   string
		pkg        string
		account    string
		amount     int64
		termMonths int
	}

	tests := []struct {
		name    string
		change  func(*input)
		wantErr error
	}{
		{
			name:   "empty application ID",
			change: func(i *input) { i.id = "" },
		},
		{
			name:   "empty application number",
			change: func(i *input) { i.number = "" },
		},
		{
			name:   "empty customer ID",
			change: func(i *input) { i.customer = "" },
		},
		{
			name:    "invalid loan package",
			change:  func(i *input) { i.pkg = "" },
			wantErr: ErrInvalidLoanPackage,
		},
		{
			name:    "invalid disbursement account",
			change:  func(i *input) { i.account = "" },
			wantErr: ErrInvalidDisbursementAccount,
		},
		{
			name:    "zero amount",
			change:  func(i *input) { i.amount = 0 },
			wantErr: ErrInvalidAmount,
		},
		{
			name:    "negative amount",
			change:  func(i *input) { i.amount = -1000 },
			wantErr: ErrInvalidAmount,
		},
		{
			name:    "zero term",
			change:  func(i *input) { i.termMonths = 0 },
			wantErr: ErrUnsupportedTerm,
		},
		{
			name:    "negative term",
			change:  func(i *input) { i.termMonths = -12 },
			wantErr: ErrUnsupportedTerm,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := input{
				id:         "app_001",
				number:     "LA20261014001",
				customer:   "customer_001",
				pkg:        "package_001",
				account:    "account_001",
				amount:     50000000,
				termMonths: 12,
			}

			tt.change(&data)

			app, err := NewDraftApplication(
				data.id,
				data.number,
				data.customer,
				data.pkg,
				data.account,
				data.amount,
				data.termMonths,
			)

			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if app != nil {
				t.Error("expected nil application")
			}

			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}
