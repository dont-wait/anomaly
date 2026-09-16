package main

import (
	"testing"

	"golang.org/x/crypto/bcrypt"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

func TestLoadSeedConfigRequiresLocalOptIn(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("SEED_DEMO_ENABLED", "true")

	if _, err := loadSeedConfig(); err == nil {
		t.Fatal("expected production seed to be refused")
	}
}

func TestLoadSeedConfigRequiresCredentials(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("SEED_DEMO_ENABLED", "true")

	if _, err := loadSeedConfig(); err == nil {
		t.Fatal("expected missing seed credentials to be refused")
	}
}

func TestLoadSeedConfigAcceptsLocalOptInAndCredentials(t *testing.T) {
	t.Setenv("APP_ENV", "local")
	t.Setenv("SEED_DEMO_ENABLED", "true")
	t.Setenv("SEED_USERNAME", "demo.customer")
	t.Setenv("SEED_CCCD", "079123456789")
	t.Setenv("SEED_EMAIL", "demo.customer@example.com")
	t.Setenv("SEED_PASSWORD", "local-only-password")

	config, err := loadSeedConfig()
	if err != nil {
		t.Fatalf("expected valid local seed config, got %v", err)
	}
	if config.Password != "local-only-password" {
		t.Fatal("expected password to be read from environment")
	}
}

func TestValidateExistingSeedRejectsCredentialCollision(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("different-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}

	seed := seedConfig{
		Username: "demo.customer",
		CCCD:     "079123456789",
		Email:    "demo.customer@example.com",
		Password: "local-only-password",
	}
	account := &accountdomain.UserAccount{
		Username:     seed.Username,
		Email:        seed.Email,
		PasswordHash: string(hash),
		Customer: &accountdomain.Customer{
			Identity: accountdomain.CustomerIdentity{Type: "cccd", Number: seed.CCCD},
		},
	}

	if err := validateExistingSeed(account, seed); err == nil {
		t.Fatal("expected credential collision to be rejected")
	}
}
