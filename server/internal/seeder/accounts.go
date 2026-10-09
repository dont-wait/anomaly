package seeder

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/application/account/commands"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/logger"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"
)

func init() {
	seeders.register("010_accounts", seedAccounts)
}

func seedAccounts(ctx context.Context, deps Dependencies) error {
	log := logger.NewLogger(zerolog.InfoLevel)
	for _, seed := range demoAccounts() {
		if err := seedAccount(ctx, deps.Accounts, seed); err != nil {
			return fmt.Errorf("account %s: %w", seed.Username, err)
		}
		log.Info().Str("username", seed.Username).Int64("balance", seed.Balance).Msg("seed account ready")
	}
	return nil
}

type accountSeed struct {
	Username       string
	Role           accountdomain.AccountRole
	CCCD           string
	Email          string
	Password       string
	IdempotencyKey string
	DOB            time.Time
	CCCDIssuedDate time.Time
	Balance        int64
}

// Demo data is public and intended only for local development.
// Add accounts here with unique identities and stable idempotency keys.
func demoAccounts() []accountSeed {
	seeds := []accountSeed{{
		Username:       "demo.customer",
		Role:           accountdomain.AccountRoleUser,
		CCCD:           "079123456789",
		Email:          "demo.customer@example.com",
		Password:       "DemoLocal@123",
		IdempotencyKey: "00000000-0000-4000-8000-000000000001",
		DOB:            time.Date(1995, 3, 20, 0, 0, 0, 0, time.UTC),
		CCCDIssuedDate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		Balance:        128540000,
	}}
	adminPassword := os.Getenv("ANOMALY_DEMO_ADMIN_PASSWORD")
	if adminPassword != "" {
		seeds = append(seeds, accountSeed{
			Username:       "admin.staff",
			Role:           accountdomain.AccountRoleAdmin,
			CCCD:           "001234567890",
			Email:          "admin.staff@example.com",
			Password:       adminPassword,
			IdempotencyKey: "00000000-0000-4000-8000-000000000002",
			DOB:            time.Date(1990, 6, 15, 0, 0, 0, 0, time.UTC),
			CCCDIssuedDate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
			Balance:        0,
		})
	}
	return seeds
}

func seedAccount(ctx context.Context, repo Repository, seed accountSeed) error {
	existing, err := repo.FindByCCCDNumber(ctx, seed.CCCD)
	if err != nil {
		return fmt.Errorf("find seed account: %w", err)
	}
	if existing != nil {
		if err := reconcileExistingSeed(ctx, repo, existing, seed); err != nil {
			return err
		}
		return nil
	}

	register := commands.NewRegisterAccountCommandHandler(repo, repo)
	account, err := register.Handle(ctx, commands.RegisterAccountCommand{
		IdempotencyKey: seed.IdempotencyKey,
		Username:       seed.Username,
		CCCDNumber:     seed.CCCD,
		CCCDIssuedDate: seed.CCCDIssuedDate,
		DOB:            seed.DOB,
		Email:          seed.Email,
		Password:       seed.Password,
	})
	if err != nil {
		return fmt.Errorf("register seed account: %w", err)
	}

	account.Balance = accountdomain.Balance{Current: seed.Balance}
	account.Role = seed.role()
	if err := repo.Save(ctx, account); err != nil {
		return fmt.Errorf("save seed balance: %w", err)
	}

	return nil
}

func reconcileExistingSeed(
	ctx context.Context,
	repo Repository,
	account *accountdomain.UserAccount,
	seed accountSeed,
) error {
	if err := validateExistingSeed(account, seed); err != nil {
		return err
	}

	if account.Balance.Current == seed.Balance && account.EffectiveRole() == seed.role() {
		return nil
	}

	account.Balance = accountdomain.Balance{Current: seed.Balance}
	account.Role = seed.role()
	account.Version++
	now := time.Now().UTC()
	account.UpdatedAt = now
	account.Customer.UpdatedAt = now
	if err := repo.Save(ctx, account); err != nil {
		return fmt.Errorf("save reconciled seed account: %w", err)
	}
	return nil
}

func validateExistingSeed(account *accountdomain.UserAccount, seed accountSeed) error {
	mismatches := make([]string, 0, 6)
	if account.Username != seed.Username {
		mismatches = append(mismatches, "username")
	}
	if account.Email != seed.Email {
		mismatches = append(mismatches, "email")
	}
	if bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(seed.Password)) != nil {
		mismatches = append(mismatches, "password")
	}
	if account.Customer == nil {
		mismatches = append(mismatches, "customer")
	} else {
		if account.Customer.Profile.FullName != seed.Username || account.Customer.Profile.Email != seed.Email {
			mismatches = append(mismatches, "customer profile")
		}
		if account.Customer.Identity.Type != "cccd" || account.Customer.Identity.Number != seed.CCCD {
			mismatches = append(mismatches, "customer identity")
		}
	}
	if len(mismatches) > 0 {
		return fmt.Errorf("existing account does not match configured seed: %s", strings.Join(mismatches, ", "))
	}
	return nil
}

func (seed accountSeed) role() accountdomain.AccountRole {
	if seed.Role == "" {
		return accountdomain.AccountRoleUser
	}
	return seed.Role
}
