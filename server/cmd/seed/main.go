package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/dont-wait/anomaly/internal/application/account/commands"
	"github.com/dont-wait/anomaly/internal/domain"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	mongo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/logger"
	"github.com/rs/zerolog"
)

const (
	seedUsername = "demo.customer"
	seedCCCD     = "079123456789"
	seedEmail    = "demo.customer@example.com"
	seedPassword = "demo-password-123"
	seedBalance  = int64(128540000)
)

func main() {
	log := logger.NewLogger(zerolog.InfoLevel)
	loader := domain.GetEnvLoader().Load(log)
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

	existing, err := repo.FindByCCCDNumber(ctx, seedCCCD)
	if err != nil {
		fatal(log, "find seed account failed", err)
	}
	if existing != nil {
		fmt.Printf("seed account already exists: username=%s cccd=%s\n", seedUsername, seedCCCD)
		return
	}

	register := commands.NewRegisterAccountCommandHandler(repo, repo)
	account, err := register.Handle(ctx, commands.RegisterAccountCommand{
		IdempotencyKey: "00000000-0000-4000-8000-000000000001",
		Username:       seedUsername,
		CCCDNumber:     seedCCCD,
		CCCDIssuedDate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		DOB:            time.Date(1995, 3, 20, 0, 0, 0, 0, time.UTC),
		Email:          seedEmail,
		Password:       seedPassword,
	})
	if err != nil {
		fatal(log, "register seed account failed", err)
	}

	account.Balance = accountdomain.Balance{Current: seedBalance}
	if err := repo.Save(ctx, account); err != nil {
		fatal(log, "save seed balance failed", err)
	}

	fmt.Printf("seeded account: username=%s cccd=%s password=%s balance=%d\n", seedUsername, seedCCCD, seedPassword, seedBalance)
}

func fatal(log *zerolog.Logger, message string, err error) {
	log.Error().Err(err).Msg(message)
	os.Exit(1)
}
