package kafka

import (
	"context"
	"testing"

	"github.com/dont-wait/anomaly/internal/domain"
	kafkago "github.com/segmentio/kafka-go"
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
	producer := &Producer{writer: validationWriter{t: t}}

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

type validationWriter struct {
	t *testing.T
}

func (w validationWriter) WriteMessages(context.Context, ...kafkago.Message) error {
	w.t.Fatal("WriteMessages must not be called for invalid events")
	return nil
}

func (validationWriter) Close() error { return nil }

func TestEnsureTopicValidatesInput(t *testing.T) {
	if err := EnsureTopic(context.Background(), nil, "topic"); err == nil {
		t.Fatal("expected error for missing brokers")
	}
	if err := EnsureTopic(context.Background(), []string{" "}, "topic"); err == nil {
		t.Fatal("expected error for blank broker")
	}
	if err := EnsureTopic(context.Background(), []string{"localhost:9092"}, " "); err == nil {
		t.Fatal("expected error for blank topic")
	}
}
