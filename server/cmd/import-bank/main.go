package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	"github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/logger"
	"github.com/rs/zerolog"
)

func main() {
	offline := flag.Bool("offline", false, "confirm API and all writers/projectors are stopped before importing legacy history")
	source := flag.String("source", "eventstore", "eventstore: migrate canonical legacy accounts; mongo: explicitly adopt audited direct-Mongo MVP state")
	flag.Parse()
	log := logger.NewLogger(zerolog.InfoLevel)
	if !*offline {
		log.Fatal().Msg("import requires -offline with all writers/projectors stopped")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	loader := domain.GetEnvLoader().Load(log)
	conf := loader.LoadMongoConfig()
	client, err := mongo.NewMongoClient(ctx, conf)
	if err != nil {
		log.Fatal().Err(err).Msg("connect mongo")
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	es, err := eventstore.NewEventStoreClient(loader.LoadEventStoreConfig())
	if err != nil {
		log.Fatal().Err(err).Msg("connect eventstore")
	}
	defer eventstore.Disconnect(es)
	bank := eventstore.NewBankRepository(es, nil)
	switch *source {
	case "eventstore":
		err = bank.ImportLegacyAccounts(ctx, client, conf.MongoDBName)
	case "mongo":
		err = bank.ImportMongo(ctx, client, conf.MongoDBName)
	default:
		log.Fatal().Msg("source must be eventstore or mongo")
	}
	if err != nil {
		log.Fatal().Err(err).Msg("import banking history")
	}
	log.Info().Msg("banking history imported; EventStoreDB is now authoritative")
}
