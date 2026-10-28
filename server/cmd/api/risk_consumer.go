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

type riskDecisionUnsigned struct {
	TransferID    string    `json:"transfer_id"`
	SourceEventID string    `json:"source_event_id"`
	Decision      string    `json:"decision"`
	IsAnomalous   bool      `json:"is_anomalous"`
	RiskScore     float64   `json:"risk_score"`
	ModelVersion  string    `json:"model_version"`
	EvaluatedAt   time.Time `json:"evaluated_at"`
}

type riskDecisionEnvelope struct {
	TransferID    string    `json:"transfer_id"`
	SourceEventID string    `json:"source_event_id"`
	Decision      string    `json:"decision"`
	IsAnomalous   *bool     `json:"is_anomalous"`
	RiskScore     *float64  `json:"risk_score"`
	ModelVersion  string    `json:"model_version"`
	EvaluatedAt   time.Time `json:"evaluated_at"`
	Signature     string    `json:"signature"`
}

func handleRiskDecision(ctx context.Context, message kafkago.Message, store riskResponseStore, log zerolog.Logger, signingKey string) error {
	decision, err := decodeRiskDecision(message.Value, signingKey)
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

func decodeRiskDecision(payload []byte, signingKey string) (risk.Decision, error) {
	var input riskDecisionEnvelope
	if err := json.Unmarshal(payload, &input); err != nil {
		return risk.Decision{}, fmt.Errorf("invalid risk response JSON: %w", err)
	}
	if strings.TrimSpace(signingKey) == "" {
		return risk.Decision{}, fmt.Errorf("risk decision signing key is not configured")
	}
	if input.IsAnomalous == nil || input.RiskScore == nil {
		return risk.Decision{}, fmt.Errorf("risk assessment fields are required")
	}
	unsigned := riskDecisionUnsigned{
		TransferID: input.TransferID, SourceEventID: input.SourceEventID, Decision: input.Decision,
		IsAnomalous: *input.IsAnomalous, RiskScore: *input.RiskScore,
		ModelVersion: input.ModelVersion, EvaluatedAt: input.EvaluatedAt,
	}
	expectedSignature := signRiskDecision(unsigned, signingKey)
	if !hmac.Equal([]byte(strings.ToLower(strings.TrimSpace(input.Signature))), []byte(expectedSignature)) {
		return risk.Decision{}, fmt.Errorf("invalid risk decision signature")
	}
	decision := risk.Decision{
		TransferID: input.TransferID, SourceEventID: input.SourceEventID, Decision: input.Decision,
		Assessment: risk.Assessment{
			Decision: input.Decision, IsAnomalous: unsigned.IsAnomalous,
			RiskScore: unsigned.RiskScore, ModelVersion: input.ModelVersion, EvaluatedAt: input.EvaluatedAt,
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

func signRiskDecision(input riskDecisionUnsigned, signingKey string) string {
	payload, _ := json.Marshal(input)
	hasher := hmac.New(sha256.New, []byte(signingKey))
	_, _ = hasher.Write(payload)
	return hex.EncodeToString(hasher.Sum(nil))
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
			return handleRiskDecision(messageCtx, message, store, log, config.RiskDecisionSigningKey)
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
