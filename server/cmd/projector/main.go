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
		position, found, err := loadCheckpoint(ctx, 2*time.Second, projection.Position, func(err error) {
			log.Error().Err(err).Msg("load banking checkpoint; retrying")
		})
		if err != nil {
			break
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
		if !waitForRetry(ctx, 2*time.Second) {
			break
		}
	}
}

func loadCheckpoint(ctx context.Context, retryDelay time.Duration, position func(context.Context) (uint64, bool, error), onError func(error)) (uint64, bool, error) {
	for {
		checkpoint, found, err := position(ctx)
		if err == nil {
			return checkpoint, found, nil
		}
		onError(err)
		if !waitForRetry(ctx, retryDelay) {
			return 0, false, ctx.Err()
		}
	}
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
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
