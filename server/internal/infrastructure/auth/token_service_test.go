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

func TestTokenServiceSeparatesAccessAndKYCPurposes(t *testing.T) {
	service := NewTokenService("test-secret", time.Hour)
	accessToken, _, err := service.Issue("account-id", "alice", "user", false)
	if err != nil {
		t.Fatal(err)
	}
	kycToken, expiresAt, err := service.IssueKYC("account-id")
	if err != nil {
		t.Fatal(err)
	}
	if time.Until(expiresAt) > 15*time.Minute || time.Until(expiresAt) < 14*time.Minute {
		t.Fatalf("KYC expiry = %v, want approximately 15 minutes", expiresAt)
	}
	if _, err := service.Parse(kycToken); err == nil {
		t.Fatal("Parse accepted a KYC token as an access token")
	}
	if _, err := service.ParseKYC(accessToken); err == nil {
		t.Fatal("ParseKYC accepted an access token")
	}
	claims, err := service.ParseKYC(kycToken)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != "account-id" || claims.Purpose != "kyc" {
		t.Fatalf("KYC claims = %#v", claims)
	}
}
