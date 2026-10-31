package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/domain/riskassessment"
	kafkainfra "github.com/dont-wait/anomaly/internal/infrastructure/kafka"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	kafkago "github.com/segmentio/kafka-go"
)

type riskMessageRejector interface {
	Reject(context.Context, string, int, int64, []byte, string) error
}

func decodeModelResponse(payload []byte, signingKey string) (riskassessment.ModelResponse, error) {
	var input struct {
		riskassessment.ModelResponse
		RiskScore      *float64 `json:"risk_score"`
		PredictedLabel *int     `json:"predicted_label"`
		Threshold      *float64 `json:"threshold"`
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
	response := input.ModelResponse
	response.RiskScore = *input.RiskScore
	response.PredictedLabel = *input.PredictedLabel
	response.Threshold = *input.Threshold
	if err := response.Validate(); err != nil {
		return riskassessment.ModelResponse{}, err
	}
	if strings.TrimSpace(signingKey) == "" {
		return riskassessment.ModelResponse{}, fmt.Errorf("model response signing key is not configured")
	}
	if err := riskassessment.Verify(response, signingKey); err != nil {
		return riskassessment.ModelResponse{}, err
	}
	return response, nil
}

func handleModelResponse(
	ctx context.Context,
	message kafkago.Message,
	store *mongorepo.RiskAssessmentRepository,
	rejector riskMessageRejector,
	signingKey string,
	log zerolog.Logger,
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

func runRiskDecisionConsumer(ctx context.Context, config *domain.KafkaConfig, store *mongorepo.RiskAssessmentRepository, log zerolog.Logger) {
	retryDelay := time.Second
	for ctx.Err() == nil {
		consumer, err := kafkainfra.NewConsumer(config.Brokers, config.RiskDecisionTopic, config.RiskDecisionGroup)
		if err != nil {
			log.Error().Err(err).Msg("create risk decision consumer failed")
			if !waitRiskConsumerRetry(ctx, retryDelay) {
				return
			}
			retryDelay = nextRiskConsumerRetry(retryDelay)
			continue
		}
		startedAt := time.Now()
		err = consumer.Consume(ctx, func(messageCtx context.Context, message kafkago.Message) error {
			return handleModelResponse(messageCtx, message, store, store, config.RiskDecisionSigningKey, log)
		})
		_ = consumer.Close()
		if ctx.Err() != nil {
			return
		}
		log.Error().Err(err).Msg("risk decision consumer stopped; retrying")
		if time.Since(startedAt) >= 30*time.Second {
			retryDelay = time.Second
		}
		if !waitRiskConsumerRetry(ctx, retryDelay) {
			return
		}
		retryDelay = nextRiskConsumerRetry(retryDelay)
	}
}

func waitRiskConsumerRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func nextRiskConsumerRetry(current time.Duration) time.Duration {
	if current >= 30*time.Second {
		return 30 * time.Second
	}
	current *= 2
	if current > 30*time.Second {
		return 30 * time.Second
	}
	return current
}
