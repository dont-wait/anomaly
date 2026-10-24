package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

type riskDecisionMessage struct {
	TransferID    string    `json:"transfer_id"`
	SourceEventID string    `json:"source_event_id"`
	Decision      string    `json:"decision"`
	IsAnomalous   bool      `json:"is_anomalous"`
	RiskScore     float64   `json:"risk_score"`
	ModelVersion  string    `json:"model_version"`
	EvaluatedAt   time.Time `json:"evaluated_at"`
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

func (p *kafkaProducer) publish(ctx context.Context, message riskDecisionMessage) error {
	if p == nil || p.writer == nil {
		return fmt.Errorf("kafka producer is not initialized")
	}
	value, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return p.writer.WriteMessages(ctx, kafkago.Message{Key: []byte(message.TransferID), Value: value})
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
