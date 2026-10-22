package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"sort"
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
	redactionKey := kafkaConfig.PayloadRedactionKey
	if redactionKey == "" {
		redactionKey = os.Getenv("JWT_SECRET")
	}
	if strings.TrimSpace(redactionKey) == "" {
		log.Fatal().Msg("KAFKA_PAYLOAD_REDACTION_KEY or JWT_SECRET is required for payload pseudonymization")
	}

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
		if err := bootstrapModelFeatures(ctx, es, featureStore, math.MaxUint64, redactionKey, log); err != nil {
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
	if err := ensureFeatureState(ctx, es, kafkaConfig.EventStoreSubscription, featureStore, redactionKey, log); err != nil {
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

		err = consume(ctx, subscription, producer, featureStore, redactionKey, log)
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

func ensureFeatureState(ctx context.Context, client *esdb.Client, group string, featureStore modelFeatureStore, redactionKey string, log *zerolog.Logger) error {
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
	return bootstrapModelFeatures(ctx, client, featureStore, checkpoint, redactionKey, log)
}

func bootstrapModelFeatures(ctx context.Context, client *esdb.Client, featureStore modelFeatureStore, throughRevision uint64, redactionKey string, log *zerolog.Logger) error {
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
		_, pending, buildErr := buildEnrichedEnvelope(ctx, recorded, featureStore, redactionKey)
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

func consume(ctx context.Context, subscription *esdb.PersistentSubscription, producer *kafkainfra.Producer, featureStore modelFeatureStore, redactionKey string, log *zerolog.Logger) error {
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
		envelope, pendingFeatureUpdate, err := buildEnrichedEnvelope(ctx, recorded, featureStore, redactionKey)
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

		if err := producer.Publish(ctx, envelope); err != nil {
			if nackErr := subscription.Nack("kafka publish failed: "+err.Error(), esdb.NackActionRetry, resolved); nackErr != nil {
				return nackErr
			}
			return err
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
			Msg("event published to kafka")
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

func buildEnvelope(recorded *esdb.RecordedEvent, redactionKey string) (kafkainfra.EventEnvelope, error) {
	payload, err := sanitizePayload(recorded.Data, redactionKey)
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

func sanitizePayload(data []byte, redactionKey string) (json.RawMessage, error) {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	redactSensitiveFields(payload, []byte(redactionKey))
	result, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func redactSensitiveFields(value any, redactionKey []byte) {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(key))
			switch {
			case sensitivePayloadKey(normalized):
				delete(current, key)
			case pseudonymousPayloadKey(normalized):
				if text, ok := child.(string); ok && strings.TrimSpace(text) != "" {
					current[key] = pseudonymizePayloadValue(text, redactionKey)
				} else {
					delete(current, key)
				}
			default:
				redactSensitiveFields(child, redactionKey)
			}
		}
	case []any:
		for _, child := range current {
			redactSensitiveFields(child, redactionKey)
		}
	}
}

func sensitivePayloadKey(key string) bool {
	if strings.HasPrefix(key, "kyc") || (strings.HasSuffix(key, "url") && strings.Contains(key, "idcard")) {
		return true
	}
	switch key {
	case "password", "passwordhash", "secret", "accesstoken", "refreshtoken",
		"account", "accounts", "customer", "session", "kycsession", "kyctoken",
		"email", "phone", "phonenumber", "address", "name", "username", "note",
		"faceimage", "selfie", "livevideourl", "idcardfronturl", "idcardbackurl",
		"media", "attachments", "document", "documents", "verification", "biometric",
		"reference", "idempotencykey", "otpexpiresat", "otpresendat":
		return true
	default:
		return false
	}
}

func pseudonymousPayloadKey(key string) bool {
	return key == "accountid" || key == "accountno" || key == "sourceaccountid" ||
		key == "destinationaccountid" || key == "deviceid" || key == "customerid"
}

func pseudonymizePayloadValue(value string, key []byte) string {
	var digest []byte
	if len(key) > 0 {
		mac := hmac.New(sha256.New, key)
		_, _ = mac.Write([]byte(value))
		digest = mac.Sum(nil)
	} else {
		sum := sha256.Sum256([]byte(value))
		digest = sum[:]
	}
	return "redacted:" + hex.EncodeToString(digest[:12])
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
