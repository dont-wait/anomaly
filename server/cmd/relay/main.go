package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/signal"
	"strings"
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

func main() {
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
			log.Error().Err(err).Str("event_id", recorded.EventID.String()).Msg("park invalid event")
			continue
		}

		if err := producer.Publish(ctx, envelope); err != nil {
			if nackErr := subscription.Nack("kafka publish failed: "+err.Error(), esdb.NackActionRetry, resolved); nackErr != nil {
				return nackErr
			}
			return err
		}
		if pendingFeatureUpdate != nil {
			if err := featureStore.ApplyState(ctx, pendingFeatureUpdate.Transaction, pendingFeatureUpdate.NextState); err != nil {
				if nackErr := subscription.Nack("model feature state update failed: "+err.Error(), esdb.NackActionRetry, resolved); nackErr != nil {
					return nackErr
				}
				return err
			}
		}
		if err := subscription.Ack(resolved); err != nil {
			return err
		}

		log.Info().
			Str("event_id", envelope.EventID).
			Str("event_type", envelope.EventType).
			Uint64("stream_revision", envelope.StreamRevision).
			Msg("event published to kafka")
	}
	return ctx.Err()
}

func buildEnvelope(recorded *esdb.RecordedEvent) (kafkainfra.EventEnvelope, error) {
	payload, err := sanitizePayload(recorded.Data)
	if err != nil {
		return kafkainfra.EventEnvelope{}, err
	}
	return kafkainfra.EventEnvelope{
		EventID:        recorded.EventID.String(),
		EventType:      recorded.EventType,
		Stream:         recorded.StreamID,
		StreamRevision: recorded.EventNumber,
		OccurredAt:     recorded.CreatedDate,
		SchemaVersion:  2,
		Payload:        payload,
	}, nil
}

func sanitizePayload(data []byte) (json.RawMessage, error) {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	redactSensitiveFields(payload)
	result, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func redactSensitiveFields(value any) {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			switch strings.ToLower(strings.ReplaceAll(key, "_", "")) {
			case "password", "passwordhash", "secret", "accesstoken", "refreshtoken":
				delete(current, key)
			default:
				redactSensitiveFields(child)
			}
		}
	case []any:
		for _, child := range current {
			redactSensitiveFields(child)
		}
	}
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
