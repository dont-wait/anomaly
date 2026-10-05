package auth

import (
	"testing"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

func TestTokenServiceRoundTripsRole(t *testing.T) {
	service := NewTokenService("test-secret", time.Hour)
	token, _, err := service.Issue("account-id", "admin.staff", string(accountdomain.AccountRoleAdmin), true)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	claims, err := service.Parse(token)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if claims.Role != string(accountdomain.AccountRoleAdmin) {
		t.Fatalf("role = %q, want admin", claims.Role)
	}
}
