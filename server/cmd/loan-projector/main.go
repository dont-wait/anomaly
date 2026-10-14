package main

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	"github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/logger"

	esdb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
	"github.com/rs/zerolog"
)

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	log := logger.NewLogger(zerolog.InfoLevel)
	loader := domain.GetEnvLoader().Load(log)
	conf := loader.LoadAllConfig()

	client, err := mongo.NewMongoClient(
		ctx,
		conf.MongoConfig,
	)
	if err != nil {
		log.Fatal().Err(err).Msg("loan Mongo connection")
	}
	defer client.Disconnect(context.Background())

	read := mongo.NewLoanReadModel(
		client,
		conf.MongoConfig.MongoDBName,
	)

	if err := read.EnsureIndexes(ctx); err != nil {
		log.Fatal().Err(err).Msg("loan indexes")
	}

	es, err := eventstore.NewEventStoreClient(
		conf.EventStoreConfig,
	)
	if err != nil {
		log.Fatal().Err(err).
			Msg("loan EventStoreDB connection")
	}
	defer eventstore.Disconnect(es)

	repo := eventstore.NewLoanRepository(es)

	for ctx.Err() == nil {
		checkpoint, found, err := read.LoadCheckpoint(ctx)
		if err != nil {
			log.Error().Err(err).Msg("loan checkpoint")
			retry(ctx)
			continue
		}

		var from esdb.AllPosition = esdb.Start{}

		if found {
			from = esdb.Position{
				Commit:  checkpoint.Commit,
				Prepare: checkpoint.Prepare,
			}
		}

		sub, err := es.SubscribeToAll(
			ctx,
			esdb.SubscribeToAllOptions{
				From: from,
			},
		)
		if err != nil {
			log.Error().Err(err).Msg("loan subscribe")
			retry(ctx)
			continue
		}

		for ctx.Err() == nil {
			msg := sub.Recv()

			if msg.SubscriptionDropped != nil {
				log.Error().
					Err(msg.SubscriptionDropped.Error).
					Msg("loan projection dropped")
				break
			}

			if msg.EventAppeared == nil {
				continue
			}

			recorded := msg.EventAppeared.Event

			if strings.HasPrefix(
				recorded.StreamID,
				"loan-application-",
			) {
				id := strings.TrimPrefix(
					recorded.StreamID,
					"loan-application-",
				)

				app, rev, err := repo.FindByIDWithRevision(
					ctx,
					id,
				)
				if err != nil {
					log.Error().
						Err(err).
						Str("loan", id).
						Msg("loan replay failed")
					break
				}

				if app == nil {
					log.Error().
						Str("loan", id).
						Msg("loan stream missing")
					break
				}

				if err := read.Save(
					ctx,
					app,
					rev,
				); err != nil {
					log.Error().
						Err(err).
						Msg("loan projection write failed")
					break
				}
			}

			err = read.SaveCheckpoint(
				ctx,
				mongo.LoanCheckpoint{
					Commit:  recorded.Position.Commit,
					Prepare: recorded.Position.Prepare,
				},
			)
			if err != nil {
				log.Error().
					Err(err).
					Msg("loan checkpoint write failed")
				break
			}
		}

		_ = sub.Close()
		retry(ctx)
	}
}

func retry(ctx context.Context) {
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
	}
}
