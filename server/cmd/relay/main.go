package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/dont-wait/anomaly/internal/domain"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	kafkainfra "github.com/dont-wait/anomaly/internal/infrastructure/kafka"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/logger"
	esdb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
	"github.com/rs/zerolog"
)

const retryDelay = 2 * time.Second

const decisionRetention = 90 * 24 * time.Hour

const decisionCleanupInterval = time.Hour

func main() {
	bootstrapOnly := flag.Bool("bootstrap-model-features", false, "rebuild MongoDB model feature state without publishing to Kafka")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log := logger.NewLogger(zerolog.InfoLevel)
	loader := domain.GetEnvLoader().Load(log)
	eventStoreConfig := loader.LoadEventStoreConfig()
	kafkaConfig := loader.LoadKafkaConfig()
	mongoConfig := loader.LoadMongoConfig()

	es, err := eventstore.NewEventStoreClient(eventStoreConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("connect eventstore for kafka relay failed")
	}
	defer eventstore.Disconnect(es)

	mongoClient, err := mongorepo.NewMongoClient(ctx, mongoConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("connect mongo for model feature state failed")
	}
	defer func() {
		disconnectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		mongorepo.Disconnect(mongoClient, disconnectCtx, cancel)
	}()
	featureStore := mongorepo.NewModelFeatureRepository(mongoClient, mongoConfig.MongoDBName)
	if err := featureStore.EnsureIndexes(ctx); err != nil {
		log.Fatal().Err(err).Msg("ensure model feature indexes failed")
	}
	if *bootstrapOnly {
		if err := featureStore.ResetForBootstrap(ctx); err != nil {
			log.Fatal().Err(err).Msg("reset model feature state failed")
		}
		if err := bootstrapModelFeatures(ctx, es, featureStore, math.MaxUint64, log); err != nil {
			log.Fatal().Err(err).Msg("bootstrap model feature state failed")
		}
		log.Info().Msg("model feature state bootstrap completed")
		return
	}

	producer, err := kafkainfra.NewProducer(kafkaConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("create kafka producer failed")
	}
	defer func() {
		if closeErr := producer.Close(); closeErr != nil {
			log.Error().Err(closeErr).Msg("close kafka producer failed")
		}
	}()

	log.Info().
		Strs("brokers", kafkaConfig.Brokers).
		Str("topic", kafkaConfig.Topic).
		Str("eventstore_stream", journal.BankStream).
		Str("eventstore_subscription", kafkaConfig.EventStoreSubscription).
		Msg("kafka relay started")

	for ctx.Err() == nil {
		if err := producer.EnsureTopic(ctx); err != nil {
			log.Error().Err(err).Msg("ensure kafka topic failed")
			if !waitForRetry(ctx) {
				break
			}
			continue
		}
		log.Info().
			Str("producer_id", kafkaConfig.ProducerID).
			Strs("brokers", kafkaConfig.Brokers).
			Str("topic", kafkaConfig.Topic).
			Msg("kafka initialized successfully")
		if err := ensurePersistentSubscription(ctx, es, kafkaConfig.EventStoreSubscription); err != nil {
			log.Error().Err(err).Msg("ensure eventstore persistent subscription failed")
			if !waitForRetry(ctx) {
				break
			}
			continue
		}
		break
	}
	if ctx.Err() != nil {
		return
	}
	if err := ensureFeatureState(ctx, es, kafkaConfig.EventStoreSubscription, featureStore, log); err != nil {
		log.Fatal().Err(err).Msg("ensure model feature state failed")
	}
	go runDecisionCleanup(ctx, es, kafkaConfig.EventStoreSubscription, featureStore, log)

	for ctx.Err() == nil {

		subscription, err := es.SubscribeToPersistentSubscription(
			ctx,
			journal.BankStream,
			kafkaConfig.EventStoreSubscription,
			esdb.SubscribeToPersistentSubscriptionOptions{BufferSize: 1},
		)
		if err != nil {
			log.Error().Err(err).Msg("subscribe to eventstore persistent subscription failed")
			if !waitForRetry(ctx) {
				break
			}
			continue
		}

		err = consume(ctx, subscription, producer, featureStore, log)
		_ = subscription.Close()
		if ctx.Err() != nil {
			break
		}
		log.Error().Err(err).Msg("kafka relay stopped; retrying")
		if !waitForRetry(ctx) {
			break
		}
	}

	log.Info().Msg("kafka relay stopped")
}

func ensurePersistentSubscription(ctx context.Context, client *esdb.Client, group string) error {
	settings := esdb.SubscriptionSettingsDefault()
	settings.CheckpointLowerBound = 1
	settings.CheckpointUpperBound = 1
	settings.MaxSubscriberCount = 1
	settings.ConsumerStrategyName = esdb.ConsumerStrategyDispatchToSingle

	err := client.CreatePersistentSubscription(ctx, journal.BankStream, group, esdb.PersistentStreamSubscriptionOptions{
		StartFrom: esdb.Start{},
		Settings:  &settings,
	})
	if err == nil {
		return nil
	}
	var eventStoreErr *esdb.Error
	if errors.As(err, &eventStoreErr) && eventStoreErr.Code() == esdb.ErrorCodeResourceAlreadyExists {
		return nil
	}
	return err
}

func ensureFeatureState(ctx context.Context, client *esdb.Client, group string, featureStore modelFeatureStore, log *zerolog.Logger) error {
	info, err := client.GetPersistentSubscriptionInfo(ctx, journal.BankStream, group, esdb.GetPersistentSubscriptionOptions{})
	if err != nil {
		return fmt.Errorf("load persistent subscription info: %w", err)
	}
	if info == nil || info.Stats == nil || info.Stats.LastCheckpointedEventRevision == nil {
		return nil
	}
	checkpoint := *info.Stats.LastCheckpointedEventRevision
	progress, found, err := featureStore.LoadProgress(ctx)
	if err != nil {
		return fmt.Errorf("load model feature progress: %w", err)
	}
	if found && progress >= checkpoint {
		return nil
	}

	log.Warn().Uint64("subscription_checkpoint", checkpoint).
		Uint64("feature_progress", progress).Bool("progress_found", found).
		Msg("model feature state is behind EventStoreDB checkpoint; rebuilding state without republishing")
	if err := featureStore.ResetForBootstrap(ctx); err != nil {
		return fmt.Errorf("reset model feature state: %w", err)
	}
	return bootstrapModelFeatures(ctx, client, featureStore, checkpoint, log)
}

func bootstrapModelFeatures(ctx context.Context, client *esdb.Client, featureStore modelFeatureStore, throughRevision uint64, log *zerolog.Logger) error {
	count := throughRevision
	if count != math.MaxUint64 {
		count++
	}
	stream, err := client.ReadStream(ctx, journal.BankStream, esdb.ReadStreamOptions{
		Direction: esdb.Forwards,
		From:      esdb.Start{},
	}, count)
	if err != nil {
		return fmt.Errorf("read banking stream for bootstrap: %w", err)
	}
	defer stream.Close()

	var events []*esdb.RecordedEvent
	for {
		resolved, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			return fmt.Errorf("receive banking event during bootstrap: %w", recvErr)
		}
		if resolved == nil || resolved.Event == nil {
			continue
		}
		events = append(events, resolved.OriginalEvent())
	}

	type bootstrapFeatureEvent struct {
		recorded   *esdb.RecordedEvent
		occurredAt time.Time
	}
	completed := make([]bootstrapFeatureEvent, 0, len(events))
	for _, recorded := range events {
		if recorded.EventType == journal.Completed {
			record, decodeErr := journal.DecodeRecord(recorded.Data, recorded.EventID.String(), recorded.EventType)
			if decodeErr != nil || record.Event == nil {
				log.Warn().Err(decodeErr).Str("event_id", recorded.EventID.String()).Msg("skip invalid completed event during bootstrap ordering")
				continue
			}
			completed = append(completed, bootstrapFeatureEvent{
				recorded:   recorded,
				occurredAt: record.Event.OccurredAt,
			})
		}
	}
	// EventStoreDB order is the delivery order, not necessarily the business
	// occurrence order. Rebuild feature state chronologically so a later stream
	// event cannot prune activity needed by an older-dated event.
	sort.SliceStable(completed, func(i, j int) bool {
		if completed[i].occurredAt.Equal(completed[j].occurredAt) {
			return completed[i].recorded.EventNumber < completed[j].recorded.EventNumber
		}
		return completed[i].occurredAt.Before(completed[j].occurredAt)
	})

	for _, item := range completed {
		recorded := item.recorded
		_, pending, buildErr := buildEnrichedEnvelope(ctx, recorded, featureStore)
		if buildErr != nil {
			var retryableErr *retryableFeatureError
			if errors.As(buildErr, &retryableErr) {
				return fmt.Errorf("prepare feature state for event %s: %w", recorded.EventID, buildErr)
			}
			log.Warn().Err(buildErr).Str("event_id", recorded.EventID.String()).Msg("skip invalid event during model feature bootstrap")
			continue
		}
		if pending != nil {
			if applyErr := featureStore.ApplyState(ctx, pending.Transaction, pending.NextState); applyErr != nil {
				return fmt.Errorf("apply feature state for event %s: %w", recorded.EventID, applyErr)
			}
		}
	}

	var lastEvent *esdb.RecordedEvent
	for _, recorded := range events {
		if lastEvent == nil || recorded.EventNumber > lastEvent.EventNumber {
			lastEvent = recorded
		}
	}
	if lastEvent != nil {
		if saveErr := featureStore.SaveProgress(ctx, lastEvent.EventID.String(), lastEvent.EventNumber); saveErr != nil {
			return fmt.Errorf("save bootstrap progress for event %s: %w", lastEvent.EventID, saveErr)
		}
	}
	log.Info().Uint64("events_processed", uint64(len(events))).Uint64("completed_events", uint64(len(completed))).Uint64("through_revision", throughRevision).Msg("model feature state rebuilt")
	return nil
}

func runDecisionCleanup(ctx context.Context, client *esdb.Client, group string, featureStore modelFeatureStore, log *zerolog.Logger) {
	ticker := time.NewTicker(decisionCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			checkpoint, found, err := loadSubscriptionCheckpoint(ctx, client, group)
			if err != nil {
				log.Warn().Err(err).Msg("load EventStoreDB checkpoint for decision cleanup failed")
				continue
			}
			if !found {
				continue
			}
			if err := featureStore.CleanupAppliedDecisions(ctx, checkpoint, time.Now().UTC().Add(-decisionRetention)); err != nil {
				log.Warn().Err(err).Msg("cleanup old model feature decisions failed")
			}
		}
	}
}

func consume(ctx context.Context, subscription *esdb.PersistentSubscription, producer *kafkainfra.Producer, featureStore modelFeatureStore, log *zerolog.Logger) error {
	for ctx.Err() == nil {
		message := subscription.Recv()
		if message.SubscriptionDropped != nil {
			return message.SubscriptionDropped.Error
		}
		if message.EventAppeared == nil || message.EventAppeared.Event == nil {
			continue
		}

		resolved := message.EventAppeared.Event
		recorded := resolved.OriginalEvent()
		if recorded.EventType != journal.Created && recorded.EventType != journal.Completed {
			// Keep feature progress aligned with the EventStoreDB stream even
			// though only created transfers are published to Kafka.
			if err := featureStore.SaveProgress(ctx, recorded.EventID.String(), recorded.EventNumber); err != nil {
				if nackErr := subscription.Nack("model feature progress update failed: "+err.Error(), esdb.NackActionRetry, resolved); nackErr != nil {
					return nackErr
				}
				return err
			}
			if err := subscription.Ack(resolved); err != nil {
				return err
			}
			log.Debug().
				Str("event_id", recorded.EventID.String()).
				Str("event_type", recorded.EventType).
				Uint64("stream_revision", recorded.EventNumber).
				Msg("event skipped; not published to kafka")
			continue
		}
		envelope, pendingFeatureUpdate, err := buildEnrichedEnvelope(ctx, recorded, featureStore)
		if err != nil {
			action := esdb.NackActionPark
			reason := "invalid event payload: " + err.Error()
			var retryableErr *retryableFeatureError
			if errors.As(err, &retryableErr) {
				action = esdb.NackActionRetry
				reason = "model feature preparation failed: " + err.Error()
			}
			if nackErr := subscription.Nack(reason, action, resolved); nackErr != nil {
				return nackErr
			}
			if action == esdb.NackActionRetry {
				return err
			}
			if progressErr := featureStore.SaveProgress(ctx, recorded.EventID.String(), recorded.EventNumber); progressErr != nil {
				return progressErr
			}
			log.Error().Err(err).Str("event_id", recorded.EventID.String()).Msg("park invalid event")
			continue
		}

		if recorded.EventType == journal.Created {
			if err := producer.Publish(ctx, envelope); err != nil {
				if nackErr := subscription.Nack("kafka publish failed: "+err.Error(), esdb.NackActionRetry, resolved); nackErr != nil {
					return nackErr
				}
				return err
			}
		}
		if pendingFeatureUpdate != nil {
			if applyErr := featureStore.ApplyState(ctx, pendingFeatureUpdate.Transaction, pendingFeatureUpdate.NextState); applyErr != nil {
				if nackErr := subscription.Nack("model feature state update failed: "+applyErr.Error(), esdb.NackActionRetry, resolved); nackErr != nil {
					return nackErr
				}
				return applyErr
			}
		}
		if err := featureStore.SaveProgress(ctx, recorded.EventID.String(), recorded.EventNumber); err != nil {
			if nackErr := subscription.Nack("model feature progress update failed: "+err.Error(), esdb.NackActionRetry, resolved); nackErr != nil {
				return nackErr
			}
			return err
		}
		if err := subscription.Ack(resolved); err != nil {
			return err
		}

		log.Info().
			Str("event_id", envelope.EventID).
			Str("event_type", envelope.EventType).
			Uint64("stream_revision", envelope.StreamRevision).
			Msg(map[bool]string{true: "event published to kafka", false: "completed event applied to feature state"}[recorded.EventType == journal.Created])
	}
	return ctx.Err()
}

func loadSubscriptionCheckpoint(ctx context.Context, client *esdb.Client, group string) (uint64, bool, error) {
	info, err := client.GetPersistentSubscriptionInfo(ctx, journal.BankStream, group, esdb.GetPersistentSubscriptionOptions{})
	if err != nil {
		return 0, false, err
	}
	if info == nil || info.Stats == nil || info.Stats.LastCheckpointedEventRevision == nil {
		return 0, false, nil
	}
	return *info.Stats.LastCheckpointedEventRevision, true, nil
}

func buildEnvelope(recorded *esdb.RecordedEvent) (kafkainfra.EventEnvelope, error) {
	if !json.Valid(recorded.Data) {
		return kafkainfra.EventEnvelope{}, fmt.Errorf("event payload must be valid JSON")
	}
	return kafkainfra.EventEnvelope{
		EventID:        recorded.EventID.String(),
		EventType:      recorded.EventType,
		Stream:         recorded.StreamID,
		StreamRevision: recorded.EventNumber,
		OccurredAt:     recorded.CreatedDate,
		SchemaVersion:  2,
		Payload:        append(json.RawMessage(nil), recorded.Data...),
	}, nil
}

func waitForRetry(ctx context.Context) bool {
	timer := time.NewTimer(retryDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
