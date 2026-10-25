package kafka

import (
	"context"
	"fmt"
	"strings"

	kafkago "github.com/segmentio/kafka-go"
)

type Consumer struct {
	reader consumerReader
}

type consumerReader interface {
	FetchMessage(context.Context) (kafkago.Message, error)
	CommitMessages(context.Context, ...kafkago.Message) error
	Close() error
}

func NewConsumer(brokers []string, topic, group string) (*Consumer, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers are required")
	}
	for _, broker := range brokers {
		if strings.TrimSpace(broker) == "" {
			return nil, fmt.Errorf("kafka broker cannot be empty")
		}
	}
	if strings.TrimSpace(topic) == "" || strings.TrimSpace(group) == "" {
		return nil, fmt.Errorf("kafka topic and consumer group are required")
	}
	return &Consumer{reader: kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:        brokers,
		Topic:          topic,
		GroupID:        group,
		MinBytes:       1,
		MaxBytes:       10e6,
		StartOffset:    kafkago.FirstOffset,
		CommitInterval: 0,
	})}, nil
}

func (c *Consumer) Consume(ctx context.Context, handler func(context.Context, kafkago.Message) error) error {
	if c == nil || c.reader == nil {
		return fmt.Errorf("kafka consumer is not initialized")
	}
	if handler == nil {
		return fmt.Errorf("kafka consumer handler is required")
	}
	for {
		message, err := c.reader.FetchMessage(ctx)
		if err != nil {
			return err
		}
		if err := handler(ctx, message); err != nil {
			return err
		}
		if err := c.reader.CommitMessages(ctx, message); err != nil {
			return err
		}
	}
}

func (c *Consumer) Close() error {
	if c == nil || c.reader == nil {
		return nil
	}
	return c.reader.Close()
}
