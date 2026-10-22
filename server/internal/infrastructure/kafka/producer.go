package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/domain/modelfeature"
	kafkago "github.com/segmentio/kafka-go"
)

type EventEnvelope struct {
	EventID        string                 `json:"event_id"`
	EventType      string                 `json:"event_type"`
	Stream         string                 `json:"stream"`
	StreamRevision uint64                 `json:"stream_revision"`
	OccurredAt     time.Time              `json:"occurred_at"`
	SchemaVersion  int                    `json:"schema_version"`
	Payload        json.RawMessage        `json:"payload"`
	ModelFeatures  *modelfeature.Features `json:"model_features,omitempty"`
}

type Producer struct {
	writer  kafkaWriter
	brokers []string
	topic   string
}

type kafkaWriter interface {
	WriteMessages(context.Context, ...kafkago.Message) error
	Close() error
}

func NewProducer(config *domain.KafkaConfig) (*Producer, error) {
	if config == nil {
		return nil, fmt.Errorf("kafka config is required")
	}
	if len(config.Brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers are required")
	}
	for _, broker := range config.Brokers {
		if strings.TrimSpace(broker) == "" {
			return nil, fmt.Errorf("kafka broker cannot be empty")
		}
	}
	if strings.TrimSpace(config.Topic) == "" {
		return nil, fmt.Errorf("kafka topic is required")
	}

	return &Producer{
		brokers: append([]string(nil), config.Brokers...),
		topic:   config.Topic,
		writer: &kafkago.Writer{
			Addr:         kafkago.TCP(config.Brokers...),
			Topic:        config.Topic,
			Balancer:     &kafkago.Hash{},
			RequiredAcks: kafkago.RequireAll,
			Async:        false,
		},
	}, nil
}

// EnsureTopic creates the development topic when it does not exist. The
// operation is idempotent and is intentionally kept in the infrastructure
// adapter so the relay remains the only server-side Kafka integration point.
func (p *Producer) EnsureTopic(ctx context.Context) error {
	if p == nil || len(p.brokers) == 0 {
		return fmt.Errorf("kafka producer is not initialized")
	}
	client := &kafkago.Client{Addr: kafkago.TCP(p.brokers[0])}
	response, err := client.CreateTopics(ctx, &kafkago.CreateTopicsRequest{
		Addr: client.Addr,
		Topics: []kafkago.TopicConfig{{
			Topic:             p.topic,
			NumPartitions:     1,
			ReplicationFactor: 1,
		}},
	})
	if err != nil {
		return fmt.Errorf("create kafka topic %q: %w", p.topic, err)
	}
	if topicErr := response.Errors[p.topic]; topicErr != nil && !errors.Is(topicErr, kafkago.TopicAlreadyExists) {
		return fmt.Errorf("create kafka topic %q: %w", p.topic, topicErr)
	}
	return nil
}

func (p *Producer) Publish(ctx context.Context, event EventEnvelope) error {
	if p == nil || p.writer == nil {
		return fmt.Errorf("kafka producer is not initialized")
	}
	if strings.TrimSpace(event.EventID) == "" {
		return fmt.Errorf("event id is required")
	}
	if strings.TrimSpace(event.EventType) == "" {
		return fmt.Errorf("event type is required")
	}
	if len(event.Payload) == 0 || !json.Valid(event.Payload) {
		return fmt.Errorf("event payload must be valid JSON")
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal kafka event: %w", err)
	}
	return p.writer.WriteMessages(ctx, kafkago.Message{
		Key:   []byte(event.EventID),
		Value: data,
		Headers: []kafkago.Header{
			{Key: "event_type", Value: []byte(event.EventType)},
			{Key: "schema_version", Value: []byte(fmt.Sprint(event.SchemaVersion))},
		},
	})
}

func (p *Producer) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}
