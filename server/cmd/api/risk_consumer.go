package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/domain/risk"
	kafkainfra "github.com/dont-wait/anomaly/internal/infrastructure/kafka"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/rs/zerolog"
	kafkago "github.com/segmentio/kafka-go"
)

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
			var input struct {
				TransferID    string    `json:"transfer_id"`
				SourceEventID string    `json:"source_event_id"`
				Decision      string    `json:"decision"`
				IsAnomalous   bool      `json:"is_anomalous"`
				RiskScore     float64   `json:"risk_score"`
				ModelVersion  string    `json:"model_version"`
				EvaluatedAt   time.Time `json:"evaluated_at"`
			}
			if err := json.Unmarshal(message.Value, &input); err != nil {
				return err
			}
			decision := risk.Decision{
				TransferID:    input.TransferID,
				SourceEventID: input.SourceEventID,
				Decision:      input.Decision,
				Assessment: risk.Assessment{
					IsAnomalous:  input.IsAnomalous,
					RiskScore:    input.RiskScore,
					ModelVersion: input.ModelVersion,
					EvaluatedAt:  input.EvaluatedAt,
				},
			}
			if decision.CreatedAt.IsZero() {
				decision.CreatedAt = time.Now().UTC()
			}
			if decision.ExpiresAt.IsZero() {
				decision.ExpiresAt = time.Now().UTC().Add(24 * time.Hour)
			}
			return store.Save(messageCtx, decision)
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
