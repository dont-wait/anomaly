package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/domain/risk"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	kafkainfra "github.com/dont-wait/anomaly/internal/infrastructure/kafka"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	kafkago "github.com/segmentio/kafka-go"
)

type riskResponseStore interface {
	Save(context.Context, risk.Decision) error
	Reject(context.Context, string, int, int64, []byte, string) error
}

func handleRiskDecision(ctx context.Context, message kafkago.Message, store riskResponseStore, log zerolog.Logger) error {
	decision, err := decodeRiskDecision(message.Value)
	if err != nil {
		if parkErr := store.Reject(ctx, message.Topic, message.Partition, message.Offset, message.Value, err.Error()); parkErr != nil {
			return fmt.Errorf("park invalid risk decision: %w", parkErr)
		}
		log.Warn().Str("topic", message.Topic).Int("partition", message.Partition).Int64("offset", message.Offset).Msg("invalid risk decision parked")
		return nil
	}
	// Storage/network failures remain retryable: never commit these offsets.
	return store.Save(ctx, decision)
}

func decodeRiskDecision(payload []byte) (risk.Decision, error) {
	var input struct {
		TransferID    string    `json:"transfer_id"`
		SourceEventID string    `json:"source_event_id"`
		Decision      string    `json:"decision"`
		IsAnomalous   bool      `json:"is_anomalous"`
		RiskScore     float64   `json:"risk_score"`
		ModelVersion  string    `json:"model_version"`
		EvaluatedAt   time.Time `json:"evaluated_at"`
	}
	if err := json.Unmarshal(payload, &input); err != nil {
		return risk.Decision{}, fmt.Errorf("invalid risk response JSON: %w", err)
	}
	decision := risk.Decision{
		TransferID: input.TransferID, SourceEventID: input.SourceEventID, Decision: input.Decision,
		Assessment: risk.Assessment{
			Decision: input.Decision, IsAnomalous: input.IsAnomalous,
			RiskScore: input.RiskScore, ModelVersion: input.ModelVersion, EvaluatedAt: input.EvaluatedAt,
		},
	}
	if err := decision.Validate(); err != nil {
		return risk.Decision{}, err
	}
	if _, err := uuid.Parse(decision.TransferID); err != nil || decision.SourceEventID != journal.EventID(decision.TransferID, journal.Created) {
		return risk.Decision{}, fmt.Errorf("invalid risk decision correlation")
	}
	return decision, nil
}

func runRiskDecisionConsumer(ctx context.Context, config *domain.KafkaConfig, store *mongorepo.RiskDecisionRepository, log zerolog.Logger) {
	for ctx.Err() == nil {
		consumer, err := kafkainfra.NewConsumer(config.Brokers, config.RiskDecisionTopic, config.RiskDecisionGroup)
		if err != nil {
			log.Error().Err(err).Msg("create risk decision consumer failed")
			if !waitRiskConsumerRetry(ctx) {
				return
			}
			continue
		}
		err = consumer.Consume(ctx, func(messageCtx context.Context, message kafkago.Message) error {
			return handleRiskDecision(messageCtx, message, store, log)
		})
		_ = consumer.Close()
		if ctx.Err() != nil {
			return
		}
		log.Error().Err(err).Msg("risk decision consumer stopped; retrying")
		if !waitRiskConsumerRetry(ctx) {
			return
		}
	}
}

func waitRiskConsumerRetry(ctx context.Context) bool {
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
