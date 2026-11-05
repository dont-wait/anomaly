// Command notifier follows the banking stream with its own checkpoint and
// turns business events into notifications via registered EventMappers.
// Run a single instance: the checkpoint is one document.
package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	notificationapp "github.com/dont-wait/anomaly/internal/application/notification"
	"github.com/dont-wait/anomaly/internal/composition"
	"github.com/dont-wait/anomaly/internal/domain"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	"github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/logger"
	esdb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
	"github.com/rs/zerolog"
)

const (
	retryDelay        = 2 * time.Second
	maxEventAttempts  = 5
	maxAttemptBackoff = 30 * time.Second
)

type checkpointStore interface {
	Position(ctx context.Context) (uint64, bool, error)
	Save(ctx context.Context, revision uint64) error
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	log := logger.NewLogger(zerolog.InfoLevel)
	loader := domain.GetEnvLoader().Load(log)
	mongoConfig := loader.LoadMongoConfig()
	client, err := mongo.NewMongoClient(ctx, mongoConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("connect mongo")
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	notifications := mongo.NewNotificationRepository(client, mongoConfig.MongoDBName)
	if err := notifications.EnsureIndexes(ctx); err != nil {
		log.Fatal().Err(err).Msg("ensure notification indexes")
	}
	failures := mongo.NewProjectionFailureRepository(client, mongoConfig.MongoDBName)
	if err := failures.EnsureIndexes(ctx); err != nil {
		log.Fatal().Err(err).Msg("ensure projection failure indexes")
	}
	checkpoint := mongo.NewNotificationCheckpoint(client, mongoConfig.MongoDBName)

	es, err := eventstore.NewEventStoreClient(loader.LoadEventStoreConfig())
	if err != nil {
		log.Fatal().Err(err).Msg("connect eventstore")
	}
	defer eventstore.Disconnect(es)

	dispatcher := composition.NewNotificationDispatcher(client, mongoConfig.MongoDBName, loader.LoadNotificationConfig())

	for ctx.Err() == nil {
		from, err := startPosition(ctx, es, checkpoint)
		if err == nil {
			var sub *esdb.Subscription
			sub, err = es.SubscribeToStream(ctx, journal.BankStream, esdb.SubscribeToStreamOptions{From: from})
			if err == nil {
				err = consume(ctx, sub, dispatcher, checkpoint, failures, log)
				_ = sub.Close()
			}
		}
		if ctx.Err() != nil {
			break
		}
		log.Error().Err(err).Msg("notification dispatcher stopped; retrying from checkpoint")
		if !waitForRetry(ctx, retryDelay) {
			break
		}
	}
}

// startPosition resumes from the checkpoint. On the very first run it starts
// at the current end of the stream: history is never backfilled.
func startPosition(ctx context.Context, es *esdb.Client, checkpoint checkpointStore) (esdb.StreamPosition, error) {
	position, found, err := checkpoint.Position(ctx)
	if err != nil {
		return nil, err
	}
	if found {
		return esdb.Revision(position), nil
	}
	last, exists, err := lastRevision(ctx, es)
	if err != nil {
		return nil, err
	}
	if !exists {
		return esdb.Start{}, nil
	}
	if err := checkpoint.Save(ctx, last); err != nil {
		return nil, err
	}
	return esdb.Revision(last), nil
}

func lastRevision(ctx context.Context, es *esdb.Client) (uint64, bool, error) {
	stream, err := es.ReadStream(ctx, journal.BankStream, esdb.ReadStreamOptions{Direction: esdb.Backwards, From: esdb.End{}}, 1)
	if err != nil {
		if isStreamNotFound(err) {
			return 0, false, nil
		}
		return 0, false, err
	}
	defer stream.Close()
	event, err := stream.Recv()
	if errors.Is(err, io.EOF) || isStreamNotFound(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return event.Event.EventNumber, true, nil
}

func isStreamNotFound(err error) bool {
	var esErr *esdb.Error
	return errors.As(err, &esErr) && esErr.Code() == esdb.ErrorCodeResourceNotFound
}

func consume(ctx context.Context, sub *esdb.Subscription, dispatcher *notificationapp.Dispatcher, checkpoint checkpointStore, failures *mongo.ProjectionFailureRepository, log *zerolog.Logger) error {
	for ctx.Err() == nil {
		message := sub.Recv()
		if message.SubscriptionDropped != nil {
			return message.SubscriptionDropped.Error
		}
		if message.EventAppeared == nil {
			continue
		}
		recorded := message.EventAppeared.Event
		if err := handleWithRetry(ctx, dispatcher, recorded, log); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Poison event: dead-letter it and move on so one bad event does
			// not stop every later notification.
			if saveErr := failures.Save(ctx, mongo.ProjectionFailure{
				EventID:     "notification:" + recorded.EventID.String(),
				StreamID:    recorded.StreamID,
				EventType:   recorded.EventType,
				EventNumber: recorded.EventNumber,
				Reason:      "notification-dispatcher",
				Error:       err.Error(),
				FailedAt:    time.Now().UTC(),
			}); saveErr != nil {
				return saveErr
			}
			log.Error().Err(err).Str("event_id", recorded.EventID.String()).Str("event_type", recorded.EventType).
				Msg("notification dispatch failed permanently; dead-lettered")
		}
		if err := checkpoint.Save(ctx, recorded.EventNumber); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func handleWithRetry(ctx context.Context, dispatcher *notificationapp.Dispatcher, recorded *esdb.RecordedEvent, log *zerolog.Logger) error {
	record, err := journal.DecodeRecord(recorded.Data, recorded.EventID.String(), recorded.EventType)
	if err != nil {
		return err // decoding never succeeds on retry
	}
	delay := time.Second
	for attempt := 1; ; attempt++ {
		err = dispatcher.Handle(ctx, record)
		if err == nil || attempt >= maxEventAttempts || ctx.Err() != nil {
			return err
		}
		log.Warn().Err(err).Int("attempt", attempt).Str("event_id", recorded.EventID.String()).Msg("notification dispatch failed; retrying")
		if !waitForRetry(ctx, delay) {
			return ctx.Err()
		}
		delay = min(delay*2, maxAttemptBackoff)
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
