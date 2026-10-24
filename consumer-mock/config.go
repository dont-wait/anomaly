package main

import (
	"fmt"
	"os"
	"strings"
)

type config struct {
	brokers       []string
	topic         string
	outputTopic   string
	consumerGroup string
	decision      string
	riskScore     float64
	modelVersion  string
}

func loadConfig() (config, error) {
	cfg := config{
		brokers:       splitCSV(envOr("KAFKA_BROKERS", "localhost:9092")),
		topic:         envOr("KAFKA_TOPIC", "anomaly.bank.events.v1"),
		outputTopic:   envOr("KAFKA_OUTPUT_TOPIC", "anomaly.bank.risk-decisions.v1"),
		consumerGroup: envOr("KAFKA_CONSUMER_GROUP", "model-pipeline"),
		decision:      strings.ToUpper(envOr("MOCK_RISK_DECISION", "ALLOW")),
		modelVersion:  envOr("MOCK_MODEL_VERSION", "mock-v1"),
	}
	if len(cfg.brokers) == 0 {
		return config{}, fmt.Errorf("KAFKA_BROKERS is required")
	}
	if cfg.topic == "" || cfg.outputTopic == "" {
		return config{}, fmt.Errorf("KAFKA_TOPIC and KAFKA_OUTPUT_TOPIC are required")
	}
	if cfg.consumerGroup == "" {
		return config{}, fmt.Errorf("KAFKA_CONSUMER_GROUP is required")
	}
	if cfg.decision != "PENDING" && cfg.decision != "ALLOW" && cfg.decision != "BLOCK" {
		return config{}, fmt.Errorf("MOCK_RISK_DECISION must be PENDING, ALLOW, or BLOCK")
	}
	if cfg.decision == "BLOCK" {
		cfg.riskScore = 0.97
	} else if cfg.decision == "ALLOW" {
		cfg.riskScore = 0.08
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
