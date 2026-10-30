package main

import "testing"

func TestLoadConfigUsesDefaults(t *testing.T) {
	t.Setenv("KAFKA_BROKERS", "")
	t.Setenv("KAFKA_TOPIC", "")
	t.Setenv("KAFKA_CONSUMER_GROUP", "")
	t.Setenv("MOCK_RISK_SCORE", "")
	t.Setenv("MOCK_THRESHOLD", "")
	t.Setenv("KAFKA_RISK_DECISION_SIGNING_KEY", "test-risk-signing-key")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if len(cfg.brokers) != 1 || cfg.brokers[0] != "localhost:9092" {
		t.Fatalf("brokers = %#v, want localhost:9092", cfg.brokers)
	}
	if cfg.topic != "anomaly.bank.events.v1" {
		t.Fatalf("topic = %q, want anomaly.bank.events.v1", cfg.topic)
	}
	if cfg.outputTopic != "anomaly.bank.risk-decisions.v1" || cfg.riskScore != 0.9821 || cfg.threshold != 0.9517361521720886 {
		t.Fatalf("model output config = %#v, want model response defaults", cfg)
	}
	if cfg.consumerGroup != "model-pipeline" {
		t.Fatalf("consumer group = %q, want model-pipeline", cfg.consumerGroup)
	}
}

func TestLoadConfigUsesEnvironment(t *testing.T) {
	t.Setenv("KAFKA_BROKERS", "kafka-a:9092, kafka-b:9092")
	t.Setenv("KAFKA_TOPIC", "events.v1")
	t.Setenv("KAFKA_CONSUMER_GROUP", "log-reader")
	t.Setenv("MOCK_RISK_SCORE", "0.1")
	t.Setenv("MOCK_THRESHOLD", "0.9")
	t.Setenv("KAFKA_RISK_DECISION_SIGNING_KEY", "test-risk-signing-key")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if len(cfg.brokers) != 2 || cfg.brokers[0] != "kafka-a:9092" || cfg.brokers[1] != "kafka-b:9092" {
		t.Fatalf("brokers = %#v, want two trimmed brokers", cfg.brokers)
	}
	if cfg.topic != "events.v1" || cfg.consumerGroup != "log-reader" {
		t.Fatalf("config = %#v, want environment values", cfg)
	}
	if cfg.riskScore != 0.1 || cfg.threshold != 0.9 {
		t.Fatalf("scores = %v/%v, want 0.1/0.9", cfg.riskScore, cfg.threshold)
	}
}

func TestNewKafkaConsumerRequiresConfiguration(t *testing.T) {
	tests := []config{
		{topic: "events", consumerGroup: "consumer"},
		{brokers: []string{" "}, topic: "events", consumerGroup: "consumer"},
		{brokers: []string{"localhost:9092"}, consumerGroup: "consumer"},
		{brokers: []string{"localhost:9092"}, topic: "events"},
	}

	for _, cfg := range tests {
		if _, err := newKafkaConsumer(cfg); err == nil {
			t.Fatalf("newKafkaConsumer(%#v) error = nil, want error", cfg)
		}
	}
}

func TestKafkaConsumerRejectsNilHandler(t *testing.T) {
	consumer, err := newKafkaConsumer(config{
		brokers:       []string{"localhost:9092"},
		topic:         "events",
		consumerGroup: "consumer",
	})
	if err != nil {
		t.Fatalf("newKafkaConsumer() error = %v", err)
	}
	defer func() { _ = consumer.close() }()

	if err := consumer.consume(nil, nil); err == nil {
		t.Fatal("consume() error = nil, want error")
	}
}
