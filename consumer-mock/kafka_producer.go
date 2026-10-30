package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

type modelResponse struct {
	SchemaVersion  int       `json:"schema_version"`
	EventID        string    `json:"event_id"`
	TransactionID  string    `json:"transaction_id"`
	OccurredAt     time.Time `json:"occurred_at"`
	ScoredAt       time.Time `json:"scored_at"`
	RiskScore      float64   `json:"risk_score"`
	PredictedLabel int       `json:"predicted_label"`
	Threshold      float64   `json:"threshold"`
	ModelVersion   string    `json:"model_version"`
	FeatureVersion string    `json:"feature_version"`
	Signature      string    `json:"signature"`
}

type kafkaProducer struct {
	writer *kafkago.Writer
}

func newKafkaProducer(cfg config) (*kafkaProducer, error) {
	if len(cfg.brokers) == 0 || strings.TrimSpace(cfg.outputTopic) == "" {
		return nil, fmt.Errorf("kafka output configuration is required")
	}
	return &kafkaProducer{writer: &kafkago.Writer{
		Addr:         kafkago.TCP(cfg.brokers...),
		Topic:        cfg.outputTopic,
		Balancer:     &kafkago.Hash{},
		RequiredAcks: kafkago.RequireAll,
		Async:        false,
	}}, nil
}

func (p *kafkaProducer) publish(ctx context.Context, input modelInput, cfg config) error {
	if p == nil || p.writer == nil {
		return fmt.Errorf("kafka producer is not initialized")
	}
	scoredAt := time.Now().UTC()
	message := modelResponse{
		SchemaVersion:  1,
		EventID:        input.EventID,
		TransactionID:  input.TransactionID,
		OccurredAt:     input.CreatedAt,
		ScoredAt:       scoredAt,
		RiskScore:      cfg.riskScore,
		PredictedLabel: boolToInt(cfg.riskScore >= cfg.threshold),
		Threshold:      cfg.threshold,
		ModelVersion:   cfg.modelVersion,
		FeatureVersion: cfg.featureVersion,
	}
	canonical, err := canonicalModelResponse(message)
	if err != nil {
		return err
	}
	hasher := hmac.New(sha256.New, []byte(cfg.signingKey))
	_, _ = hasher.Write(canonical)
	message.Signature = hex.EncodeToString(hasher.Sum(nil))
	value, err := json.Marshal(message)
	if err != nil {
		return err
	}
	key := strings.Join([]string{message.TransactionID, message.ModelVersion, message.FeatureVersion}, ":")
	return p.writer.WriteMessages(ctx, kafkago.Message{Key: []byte(key), Value: value})
}

func canonicalModelResponse(message modelResponse) ([]byte, error) {
	return json.Marshal(map[string]any{
		"event_id":        message.EventID,
		"occurred_at":     message.OccurredAt,
		"predicted_label": message.PredictedLabel,
		"risk_score":      message.RiskScore,
		"schema_version":  message.SchemaVersion,
		"scored_at":       message.ScoredAt,
		"threshold":       message.Threshold,
		"transaction_id":  message.TransactionID,
		"model_version":   message.ModelVersion,
		"feature_version": message.FeatureVersion,
	})
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (p *kafkaProducer) ensureTopic(ctx context.Context) error {
	if p == nil || p.writer == nil {
		return fmt.Errorf("kafka producer is not initialized")
	}
	client := &kafkago.Client{Addr: p.writer.Addr}
	response, err := client.CreateTopics(ctx, &kafkago.CreateTopicsRequest{
		Addr:   client.Addr,
		Topics: []kafkago.TopicConfig{{Topic: p.writer.Topic, NumPartitions: 1, ReplicationFactor: 1}},
	})
	if err != nil {
		return err
	}
	if topicErr := response.Errors[p.writer.Topic]; topicErr != nil && !strings.Contains(topicErr.Error(), "already exists") {
		return topicErr
	}
	return nil
}

func (p *kafkaProducer) close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}
