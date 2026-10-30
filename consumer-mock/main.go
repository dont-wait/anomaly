package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := loadConfig()
	if err != nil {
		log.Error("load config failed", "error", err)
		os.Exit(1)
	}

	producer, err := newKafkaProducer(cfg)
	if err != nil {
		log.Error("create kafka producer failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = producer.close() }()
	if err := producer.ensureTopic(ctx); err != nil {
		log.Warn("ensure kafka output topic failed; topic will be retried on publish", "error", err)
	}

	log.Info("kafka model consumer starting", "brokers", cfg.brokers, "topic", cfg.topic, "output_topic", cfg.outputTopic, "consumer_group", cfg.consumerGroup, "risk_score", cfg.riskScore, "threshold", cfg.threshold, "model_version", cfg.modelVersion, "feature_version", cfg.featureVersion)
	for ctx.Err() == nil {
		consumer, err := newKafkaConsumer(cfg)
		if err != nil {
			log.Error("create kafka consumer failed", "error", err)
			if !waitForRetry(ctx, time.Second) {
				break
			}
			continue
		}

		err = consumer.consume(ctx, modelHandler(log, producer, cfg))
		if closeErr := consumer.close(); closeErr != nil && ctx.Err() == nil {
			log.Warn("close kafka consumer failed", "error", closeErr)
		}
		if ctx.Err() != nil {
			break
		}

		log.Error("kafka consumer stopped; retrying", "error", err)
		if !waitForRetry(ctx, 2*time.Second) {
			break
		}
	}

	log.Info("kafka consumer stopped")
}

func modelHandler(log *slog.Logger, producer *kafkaProducer, cfg config) messageHandler {
	return func(ctx context.Context, message kafkago.Message) error {
		var metadata struct {
			EventID       string `json:"event_id"`
			EventType     string `json:"event_type"`
			SchemaVersion int    `json:"schema_version"`
		}
		if err := json.Unmarshal(message.Value, &metadata); err != nil {
			return fmt.Errorf("decode kafka event metadata: %w", err)
		}
		if metadata.EventType != modelEventType {
			// Ignore historical or unrelated events. Only created transfers are
			// model inputs in the pre-completion risk flow.
			return nil
		}

		if input, ok, err := selectModelInput(message.Value); err != nil {
			return err
		} else if ok {
			log.Info("model fields selected",
				"topic", message.Topic,
				"partition", message.Partition,
				"offset", message.Offset,
				"transaction_id", input.TransactionID,
				"amount", input.Amount,
				"channel", input.Channel,
				"avg_amount_7d", input.AvgAmount7d,
				"tx_count_1h", input.TxCount1h,
				"tx_count_24h", input.TxCount24h,
				"last_tx_at", input.LastTxAt,
				"new_receiver", input.NewReceiver,
				"new_device", input.NewDevice,
			)
			return producer.publish(ctx, input, cfg)
		}
		return fmt.Errorf("model input was not selected")
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
