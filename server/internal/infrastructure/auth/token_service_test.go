package auth

import (
	"testing"
	"time"
)

func TestTokenServiceRoundTripsRole(t *testing.T) {
	service := NewTokenService("test-secret", time.Hour)
	token, _, err := service.Issue("account-id", "admin.staff", "admin", true)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	claims, err := service.Parse(token)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if claims.Role != "admin" {
		t.Fatalf("role = %q, want admin", claims.Role)
	}
}
