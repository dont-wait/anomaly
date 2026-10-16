package kafka

import (
	"context"
	"testing"

	"github.com/dont-wait/anomaly/internal/domain"
)

func TestNewProducerRequiresConfiguration(t *testing.T) {
	tests := []*domain.KafkaConfig{
		nil,
		{},
		{Brokers: []string{" "}, Topic: "events"},
		{Brokers: []string{"localhost:9092"}},
	}
	for _, config := range tests {
		if _, err := NewProducer(config); err == nil {
			t.Fatalf("NewProducer(%#v) error = nil, want error", config)
		}
	}
}

func TestProducerPublishValidatesEvent(t *testing.T) {
	producer, err := NewProducer(&domain.KafkaConfig{
		Brokers: []string{"localhost:9092"},
		Topic:   "events",
	})
	if err != nil {
		t.Fatalf("NewProducer() error = %v", err)
	}
	defer func() { _ = producer.Close() }()

	for _, event := range []EventEnvelope{
		{EventType: "TestEvent", Payload: []byte(`{}`)},
		{EventID: "event-1", Payload: []byte(`{}`)},
		{EventID: "event-1", EventType: "TestEvent", Payload: []byte(`not-json`)},
	} {
		if err := producer.Publish(context.Background(), event); err == nil {
			t.Fatalf("Publish(%#v) error = nil, want validation error", event)
		}
	}
}
