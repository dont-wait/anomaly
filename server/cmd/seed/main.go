package main

import (
	"context"
	"os"
	"time"

	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	"github.com/dont-wait/anomaly/internal/logger"
	"github.com/dont-wait/anomaly/internal/seeder"
	"github.com/rs/zerolog"
)

func main() {
	log := logger.NewLogger(zerolog.InfoLevel)
	loader := domain.GetEnvLoader().Load(log)
	if err := seeder.ValidateEnvironment(os.Getenv("APP_ENV"), os.Getenv("SEED_DEMO_ENABLED")); err != nil {
		log.Fatal().Err(err).Msg("seed refused")
	}
	config := loader.LoadEventStoreConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := eventstore.NewEventStoreClient(config)
	if err != nil {
		log.Fatal().Err(err).Msg("connect eventstore failed")
	}
	defer eventstore.Disconnect(client)
	repo := eventstore.SeedRepository{BankRepository: eventstore.NewBankRepository(client, nil)}

	if err := seeder.Run(ctx, seeder.Dependencies{Accounts: repo}); err != nil {
		log.Fatal().Err(err).Msg("seed failed")
	}
	log.Info().Msg("all seeders completed")
}
