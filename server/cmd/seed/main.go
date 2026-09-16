package main

import (
	"context"
	"os"
	"time"

	"github.com/dont-wait/anomaly/internal/domain"
	mongo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
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
	config := loader.LoadMongoConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := mongo.NewMongoClient(ctx, config)
	if err != nil {
		log.Fatal().Err(err).Msg("connect mongo failed")
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			log.Error().Err(err).Msg("disconnect mongo failed")
		}
	}()

	repo := mongo.NewAccountAggregateRepository(client, config.MongoDBName)
	if err := repo.EnsureIndexes(ctx); err != nil {
		log.Fatal().Err(err).Msg("ensure indexes failed")
	}

	if err := seeder.Run(ctx, seeder.Dependencies{Accounts: repo}); err != nil {
		log.Fatal().Err(err).Msg("seed failed")
	}
	log.Info().Msg("all seeders completed")
}
