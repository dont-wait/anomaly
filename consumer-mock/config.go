package main

import (
	"fmt"
	"os"
	"strings"
)

type config struct {
	brokers       []string
	topic         string
	consumerGroup string
}

func loadConfig() (config, error) {
	cfg := config{
		brokers:       splitCSV(envOr("KAFKA_BROKERS", "localhost:9092")),
		topic:         envOr("KAFKA_TOPIC", "anomaly.bank.events.v1"),
		consumerGroup: envOr("KAFKA_CONSUMER_GROUP", "model-pipeline"),
	}
	if len(cfg.brokers) == 0 {
		return config{}, fmt.Errorf("KAFKA_BROKERS is required")
	}
	if cfg.topic == "" {
		return config{}, fmt.Errorf("KAFKA_TOPIC is required")
	}
	if cfg.consumerGroup == "" {
		return config{}, fmt.Errorf("KAFKA_CONSUMER_GROUP is required")
	}
	return cfg, nil
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}
