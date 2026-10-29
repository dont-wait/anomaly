package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/domain/risk"
	"github.com/dont-wait/anomaly/internal/domain/riskassessment"
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

type riskMessageRejector interface {
	Reject(context.Context, string, int, int64, []byte, string) error
}

type riskAssessmentStore interface {
	Save(context.Context, riskassessment.Assessment) (bool, error)
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
	if err := store.Save(ctx, decision); err != nil {
		return err
	}
	log.Info().Str("transfer_id", decision.TransferID).Str("decision", decision.Decision).
		Msg("risk decision processed")
	return nil
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

func isModelResponse(payload []byte) bool {
	var envelope struct {
		TransactionID  string `json:"transaction_id"`
		PredictedLabel *int   `json:"predicted_label"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return false
	}
	return envelope.TransactionID != "" || envelope.PredictedLabel != nil
}

func decodeModelResponse(payload []byte, signingKey string) (riskassessment.ModelResponse, error) {
	var input struct {
		TransactionID  string   `json:"transaction_id"`
		RiskScore      *float64 `json:"risk_score"`
		PredictedLabel *int     `json:"predicted_label"`
		Threshold      *float64 `json:"threshold"`
		ModelVersion   string   `json:"model_version"`
		FeatureVersion string   `json:"feature_version"`
		Signature      string   `json:"signature"`
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return riskassessment.ModelResponse{}, fmt.Errorf("invalid model response JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return riskassessment.ModelResponse{}, fmt.Errorf("model response contains trailing data")
		}
		return riskassessment.ModelResponse{}, fmt.Errorf("invalid trailing model response data: %w", err)
	}
	if input.RiskScore == nil || input.PredictedLabel == nil || input.Threshold == nil {
		return riskassessment.ModelResponse{}, fmt.Errorf("risk_score, predicted_label and threshold are required and cannot be null")
	}
	response := riskassessment.ModelResponse{
		TransactionID: input.TransactionID, RiskScore: *input.RiskScore,
		PredictedLabel: *input.PredictedLabel, Threshold: *input.Threshold,
		ModelVersion: input.ModelVersion, FeatureVersion: input.FeatureVersion,
	}
	if err := response.Validate(); err != nil {
		return riskassessment.ModelResponse{}, err
	}
	if strings.TrimSpace(signingKey) == "" {
		return riskassessment.ModelResponse{}, fmt.Errorf("risk decision signing key is not configured")
	}
	expectedSignature := signModelResponse(response, signingKey)
	if !hmac.Equal([]byte(strings.ToLower(strings.TrimSpace(input.Signature))), []byte(expectedSignature)) {
		return riskassessment.ModelResponse{}, fmt.Errorf("invalid model response signature")
	}
	return response, nil
}

func signModelResponse(response riskassessment.ModelResponse, signingKey string) string {
	payload, _ := json.Marshal(response)
	hasher := hmac.New(sha256.New, []byte(signingKey))
	_, _ = hasher.Write(payload)
	return hex.EncodeToString(hasher.Sum(nil))
}

func handleModelResponse(
	ctx context.Context,
	message kafkago.Message,
	store riskAssessmentStore,
	rejector riskMessageRejector,
	log zerolog.Logger,
	signingKey string,
) error {
	response, err := decodeModelResponse(message.Value, signingKey)
	if err != nil {
		if parkErr := rejector.Reject(ctx, message.Topic, message.Partition, message.Offset, message.Value, err.Error()); parkErr != nil {
			return fmt.Errorf("park invalid model response: %w", parkErr)
		}
		log.Warn().Str("topic", message.Topic).Int("partition", message.Partition).Int64("offset", message.Offset).Msg("invalid model response parked")
		return nil
	}

	assessment, err := riskassessment.NewAssessment(
		response,
		time.Now().UTC(),
		message.Topic,
		message.Partition,
		message.Offset,
		uuid.NewString(),
	)
	if err != nil {
		return err
	}
	inserted, err := store.Save(ctx, assessment)
	if err != nil {
		if errors.Is(err, mongorepo.ErrRiskAssessmentConflict) {
			if parkErr := rejector.Reject(ctx, message.Topic, message.Partition, message.Offset, message.Value, err.Error()); parkErr != nil {
				return fmt.Errorf("park conflicting model response: %w", parkErr)
			}
			log.Warn().Str("transactionId", response.TransactionID).Msg("conflicting model response parked")
			return nil
		}
		return err
	}
	log.Info().Str("transaction_id", response.TransactionID).
		Str("model_version", response.ModelVersion).Str("feature_version", response.FeatureVersion).
		Bool("inserted", inserted).Msg("model assessment stored")
	return nil
}

func handleRiskResponse(ctx context.Context, message kafkago.Message, decisions riskResponseStore, assessments riskAssessmentStore, log zerolog.Logger, signingKey string) error {
	if isModelResponse(message.Value) {
		return handleModelResponse(ctx, message, assessments, decisions, log, signingKey)
	}
	return handleRiskDecision(ctx, message, decisions, log, signingKey)
}

func signRiskDecision(input riskDecisionUnsigned, signingKey string) string {
	payload, _ := json.Marshal(input)
	hasher := hmac.New(sha256.New, []byte(signingKey))
	_, _ = hasher.Write(payload)
	return hex.EncodeToString(hasher.Sum(nil))
}

func runRiskDecisionConsumer(ctx context.Context, config *domain.KafkaConfig, store *mongorepo.RiskDecisionRepository, assessments *mongorepo.RiskAssessmentRepository, log zerolog.Logger) {
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
			return handleRiskResponse(messageCtx, message, store, assessments, log, config.RiskDecisionSigningKey)
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
