package main

import (
	"context"
	"os"
	"time"

	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
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
	eventStoreConfig := loader.LoadEventStoreConfig()
	mongoConfig := loader.LoadMongoConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := eventstore.NewEventStoreClient(eventStoreConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("connect eventstore failed")
	}
	defer eventstore.Disconnect(client)
	repo := eventstore.SeedRepository{BankRepository: eventstore.NewBankRepository(client, nil)}
	mongoClient, err := mongo.NewMongoClient(ctx, mongoConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("connect mongo failed")
	}
	defer func() { _ = mongoClient.Disconnect(ctx) }()
	loanCatalog := mongo.NewLoanPackageCatalog(mongoClient, mongoConfig.MongoDBName)
	if err := loanCatalog.EnsureIndexes(ctx); err != nil {
		log.Fatal().Err(err).Msg("ensure loan catalog indexes failed")
	}

	if err := seeder.Run(ctx, seeder.Dependencies{Accounts: repo, LoanCatalog: loanCatalog}); err != nil {
		log.Fatal().Err(err).Msg("seed failed")
	}
	log.Info().Msg("all seeders completed")
}
