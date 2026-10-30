package main

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

type config struct {
	brokers        []string
	topic          string
	outputTopic    string
	consumerGroup  string
	signingKey     string
	riskScore      float64
	threshold      float64
	modelVersion   string
	featureVersion string
}

func loadConfig() (config, error) {
	cfg := config{
		brokers:        splitCSV(envOr("KAFKA_BROKERS", "localhost:9092")),
		topic:          envOr("KAFKA_TOPIC", "anomaly.bank.events.v1"),
		outputTopic:    envOr("KAFKA_OUTPUT_TOPIC", "anomaly.bank.risk-decisions.v1"),
		consumerGroup:  envOr("KAFKA_CONSUMER_GROUP", "model-pipeline"),
		signingKey:     envOr("KAFKA_RISK_DECISION_SIGNING_KEY", ""),
		riskScore:      0.9821,
		threshold:      0.9517361521720886,
		modelVersion:   envOr("MOCK_MODEL_VERSION", "gatv2-device-control-v1"),
		featureVersion: envOr("MOCK_FEATURE_VERSION", "schema-3-0b9efd44d793"),
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
	if cfg.signingKey == "" {
		return config{}, fmt.Errorf("KAFKA_RISK_DECISION_SIGNING_KEY is required")
	}
	var err error
	if value := strings.TrimSpace(os.Getenv("MOCK_RISK_SCORE")); value != "" {
		cfg.riskScore, err = strconv.ParseFloat(value, 64)
		if err != nil {
			return config{}, fmt.Errorf("MOCK_RISK_SCORE must be a number: %w", err)
		}
	}
	if value := strings.TrimSpace(os.Getenv("MOCK_THRESHOLD")); value != "" {
		cfg.threshold, err = strconv.ParseFloat(value, 64)
		if err != nil {
			return config{}, fmt.Errorf("MOCK_THRESHOLD must be a number: %w", err)
		}
	}
	if math.IsNaN(cfg.riskScore) || math.IsInf(cfg.riskScore, 0) || cfg.riskScore < 0 || cfg.riskScore > 1 {
		return config{}, fmt.Errorf("MOCK_RISK_SCORE must be between 0 and 1")
	}
	if math.IsNaN(cfg.threshold) || math.IsInf(cfg.threshold, 0) || cfg.threshold < 0 || cfg.threshold > 1 {
		return config{}, fmt.Errorf("MOCK_THRESHOLD must be between 0 and 1")
	}
	if cfg.modelVersion == "" || cfg.featureVersion == "" {
		return config{}, fmt.Errorf("MOCK_MODEL_VERSION and MOCK_FEATURE_VERSION are required")
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
