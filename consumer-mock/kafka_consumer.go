package main

import (
	"context"
	"fmt"
	"strings"

	kafkago "github.com/segmentio/kafka-go"
)

type kafkaConsumer struct {
	reader *kafkago.Reader
}

func newKafkaConsumer(cfg config) (*kafkaConsumer, error) {
	if len(cfg.brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers are required")
	}
	for _, broker := range cfg.brokers {
		if strings.TrimSpace(broker) == "" {
			return nil, fmt.Errorf("kafka broker cannot be empty")
		}
	}
	if strings.TrimSpace(cfg.topic) == "" {
		return nil, fmt.Errorf("kafka topic is required")
	}
	if strings.TrimSpace(cfg.consumerGroup) == "" {
		return nil, fmt.Errorf("kafka consumer group is required")
	}

	return &kafkaConsumer{
		reader: kafkago.NewReader(kafkago.ReaderConfig{
			Brokers:        cfg.brokers,
			GroupID:        cfg.consumerGroup,
			Topic:          cfg.topic,
			MinBytes:       1,
			MaxBytes:       10e6,
			StartOffset:    kafkago.FirstOffset,
			CommitInterval: 0,
		}),
	}, nil
}

type messageHandler func(context.Context, kafkago.Message) error

// consume commits an offset only after the handler succeeds. If the process
// stops before the commit, Kafka will deliver the message again after restart.
func (c *kafkaConsumer) consume(ctx context.Context, handler messageHandler) error {
	if handler == nil {
		return fmt.Errorf("kafka message handler is required")
	}

	for {
		message, err := c.reader.FetchMessage(ctx)
		if err != nil {
			return err
		}
		if err := handler(ctx, message); err != nil {
			return fmt.Errorf("handle kafka message at offset %d: %w", message.Offset, err)
		}
		if err := c.reader.CommitMessages(ctx, message); err != nil {
			return fmt.Errorf("commit kafka message at offset %d: %w", message.Offset, err)
		}
	}
}

func (c *kafkaConsumer) close() error {
	if c == nil || c.reader == nil {
		return nil
	}
	return c.reader.Close()
}
