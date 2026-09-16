package seeder

import (
	"context"
	"errors"
	"fmt"
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
	return []accountSeed{{
		Username:       "demo.customer",
		CCCD:           "079123456789",
		Email:          "demo.customer@example.com",
		Password:       "DemoLocal@123",
		IdempotencyKey: "00000000-0000-4000-8000-000000000001",
		DOB:            time.Date(1995, 3, 20, 0, 0, 0, 0, time.UTC),
		CCCDIssuedDate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		Balance:        128540000,
	}}
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

	if account.Balance.Current == seed.Balance {
		return nil
	}

	account.Balance = accountdomain.Balance{Current: seed.Balance}
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
	if account.Username != seed.Username || account.Email != seed.Email || account.Customer == nil ||
		account.Customer.Profile.FullName != seed.Username || account.Customer.Profile.Email != seed.Email ||
		account.Customer.Identity.Type != "cccd" || account.Customer.Identity.Number != seed.CCCD ||
		bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(seed.Password)) != nil {
		return errors.New("existing account does not match configured seed credentials")
	}
	return nil
}
