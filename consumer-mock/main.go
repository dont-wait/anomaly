package main

import (
	"context"
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

	log.Info("kafka consumer starting", "brokers", cfg.brokers, "topic", cfg.topic, "consumer_group", cfg.consumerGroup)
	for ctx.Err() == nil {
		consumer, err := newKafkaConsumer(cfg)
		if err != nil {
			log.Error("create kafka consumer failed", "error", err)
			if !waitForRetry(ctx, time.Second) {
				break
			}
			continue
		}

		err = consumer.consume(ctx, logMessage(log))
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

func logMessage(log *slog.Logger) messageHandler {
	return func(_ context.Context, message kafkago.Message) error {
		log.Info("kafka event received",
			"topic", message.Topic,
			"partition", message.Partition,
			"offset", message.Offset,
			"key", string(message.Key),
			"value", string(message.Value),
		)
		return nil
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
