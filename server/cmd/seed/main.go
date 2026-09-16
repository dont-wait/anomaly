package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/dont-wait/anomaly/internal/application/account/commands"
	"github.com/dont-wait/anomaly/internal/domain"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	mongo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/logger"
	"github.com/rs/zerolog"
)

const (
	seedBalance = int64(128540000)
)

type seedConfig struct {
	Username string
	CCCD     string
	Email    string
	Password string
}

func main() {
	log := logger.NewLogger(zerolog.InfoLevel)
	loader := domain.GetEnvLoader().Load(log)
	seed, err := loadSeedConfig()
	if err != nil {
		fatal(log, "seed refused", err)
	}
	config := loader.LoadMongoConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := mongo.NewMongoClient(ctx, config)
	if err != nil {
		fatal(log, "connect mongo failed", err)
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			log.Error().Err(err).Msg("disconnect mongo failed")
		}
	}()

	repo := mongo.NewAccountAggregateRepository(client, config.MongoDBName)
	if err := repo.EnsureIndexes(ctx); err != nil {
		fatal(log, "ensure indexes failed", err)
	}

	existing, err := repo.FindByCCCDNumber(ctx, seed.CCCD)
	if err != nil {
		fatal(log, "find seed account failed", err)
	}
	if existing != nil {
		if err := reconcileExistingSeed(ctx, repo, existing, seed); err != nil {
			fatal(log, "existing account does not match seed identity", err)
		}
		fmt.Printf("seed account ready: username=%s balance=%d\n", seed.Username, seedBalance)
		return
	}

	register := commands.NewRegisterAccountCommandHandler(repo, repo)
	account, err := register.Handle(ctx, commands.RegisterAccountCommand{
		IdempotencyKey: "00000000-0000-4000-8000-000000000001",
		Username:       seed.Username,
		CCCDNumber:     seed.CCCD,
		CCCDIssuedDate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		DOB:            time.Date(1995, 3, 20, 0, 0, 0, 0, time.UTC),
		Email:          seed.Email,
		Password:       seed.Password,
	})
	if err != nil {
		fatal(log, "register seed account failed", err)
	}

	account.Balance = accountdomain.Balance{Current: seedBalance}
	if err := repo.Save(ctx, account); err != nil {
		fatal(log, "save seed balance failed", err)
	}

	fmt.Printf("seeded account: username=%s balance=%d\n", seed.Username, seedBalance)
}

func reconcileExistingSeed(
	ctx context.Context,
	repo *mongo.AccountAggregateRepository,
	account *accountdomain.UserAccount,
	seed seedConfig,
) error {
	if err := validateExistingSeed(account, seed); err != nil {
		return err
	}

	if account.Balance.Current == seedBalance {
		return nil
	}

	account.Balance = accountdomain.Balance{Current: seedBalance}
	account.Version++
	now := time.Now().UTC()
	account.UpdatedAt = now
	account.Customer.UpdatedAt = now
	if err := repo.Save(ctx, account); err != nil {
		return fmt.Errorf("save reconciled seed account: %w", err)
	}
	return nil
}

func validateExistingSeed(account *accountdomain.UserAccount, seed seedConfig) error {
	if account.Username != seed.Username || account.Email != seed.Email || account.Customer == nil ||
		account.Customer.Profile.FullName != seed.Username || account.Customer.Profile.Email != seed.Email ||
		account.Customer.Identity.Type != "cccd" || account.Customer.Identity.Number != seed.CCCD ||
		bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(seed.Password)) != nil {
		return errors.New("existing account does not match configured seed credentials")
	}
	return nil
}

func loadSeedConfig() (seedConfig, error) {
	environment := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	if environment != "development" && environment != "dev" && environment != "local" {
		return seedConfig{}, fmt.Errorf("APP_ENV must be development, dev, or local; got %q", environment)
	}
	if strings.ToLower(strings.TrimSpace(os.Getenv("SEED_DEMO_ENABLED"))) != "true" {
		return seedConfig{}, errors.New("SEED_DEMO_ENABLED=true is required")
	}

	config := seedConfig{
		Username: strings.TrimSpace(os.Getenv("SEED_USERNAME")),
		CCCD:     strings.TrimSpace(os.Getenv("SEED_CCCD")),
		Email:    strings.TrimSpace(os.Getenv("SEED_EMAIL")),
		Password: os.Getenv("SEED_PASSWORD"),
	}
	if config.Username == "" || config.CCCD == "" || config.Email == "" || config.Password == "" {
		return seedConfig{}, errors.New("SEED_USERNAME, SEED_CCCD, SEED_EMAIL, and SEED_PASSWORD are required")
	}
	return config, nil
}

func fatal(log *zerolog.Logger, message string, err error) {
	log.Error().Err(err).Msg(message)
	os.Exit(1)
}
