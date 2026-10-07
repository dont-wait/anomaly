package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dont-wait/anomaly/internal/domain"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	"github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/logger"
	esdb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
	"github.com/rs/zerolog"
)

func main() {
	replay := flag.Bool("replay", false, "reset banking projection checkpoint and replay; stop other projectors first")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	log := logger.NewLogger(zerolog.InfoLevel)
	loader := domain.GetEnvLoader().Load(log)
	conf := loader.LoadMongoConfig()
	client, err := mongo.NewMongoClient(ctx, conf)
	if err != nil {
		log.Fatal().Err(err).Msg("connect mongo")
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	projection := mongo.NewBankProjection(client, conf.MongoDBName)
	if *replay {
		if err := projection.Reset(ctx); err != nil {
			log.Fatal().Err(err).Msg("reset banking checkpoint")
		}
	}
	es, err := eventstore.NewEventStoreClient(loader.LoadEventStoreConfig())
	if err != nil {
		log.Fatal().Err(err).Msg("connect eventstore")
	}
	defer eventstore.Disconnect(es)
	for ctx.Err() == nil {
		position, found, err := projection.Position(ctx)
		if err != nil {
			log.Fatal().Err(err).Msg("load banking checkpoint")
		}
		var from esdb.StreamPosition = esdb.Start{}
		if found {
			from = esdb.Revision(position)
		}
		sub, err := es.SubscribeToStream(ctx, journal.BankStream, esdb.SubscribeToStreamOptions{From: from})
		if err == nil {
			err = consume(ctx, sub, projection)
			_ = sub.Close()
		}
		if ctx.Err() != nil {
			break
		}
		log.Error().Err(err).Msg("bank projection stopped; retrying committed checkpoint")
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
}

func consume(ctx context.Context, sub *esdb.Subscription, projection *mongo.BankProjection) error {
	for ctx.Err() == nil {
		message := sub.Recv()
		if message.SubscriptionDropped != nil {
			return message.SubscriptionDropped.Error
		}
		if message.EventAppeared == nil {
			continue
		}
		recorded := message.EventAppeared.Event
		record, err := journal.DecodeRecord(recorded.Data, recorded.EventID.String(), recorded.EventType)
		if err != nil {
			return err
		}
		if err := projection.Apply(ctx, record, recorded.EventNumber); err != nil {
			return err
		}
	}
	return ctx.Err()
}
