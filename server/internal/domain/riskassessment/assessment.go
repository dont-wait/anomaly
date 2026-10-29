package riskassessment

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/dont-wait/anomaly/internal/domain/risk"
	"strings"
	"time"
)

const (
	RiskLevelNormal  = "NORMAL"
	RiskLevelWarning = "WARNING"
	RiskLevelBlock   = "BLOCK"
	RiskBlockScore   = 0.7
)

func LevelForScore(score float64) string {
	if score > RiskBlockScore {
		return RiskLevelBlock
	}
	if score >= 0.5 {
		return RiskLevelWarning
	}
	return RiskLevelNormal
}

func DecisionForScore(score float64) string {
	if score > RiskBlockScore {
		return risk.DecisionBlock
	}
	return risk.DecisionAllow
}

// ModelResponse is the scoring contract emitted by the ML service. It is an
// assessment only; predicted_label must not be treated as an ALLOW/BLOCK
// business decision.
type ModelResponse struct {
	SchemaVersion  int       `json:"schema_version" bson:"schema_version"`
	EventID        string    `json:"event_id" bson:"event_id"`
	TransactionID  string    `json:"transaction_id" bson:"transaction_id"`
	OccurredAt     time.Time `json:"occurred_at" bson:"occurred_at"`
	ScoredAt       time.Time `json:"scored_at" bson:"scored_at"`
	RiskScore      float64   `json:"risk_score" bson:"risk_score"`
	PredictedLabel int       `json:"predicted_label" bson:"predicted_label"`
	Threshold      float64   `json:"threshold" bson:"threshold"`
	ModelVersion   string    `json:"model_version" bson:"model_version"`
	FeatureVersion string    `json:"feature_version" bson:"feature_version"`
	Signature      string    `json:"signature" bson:"signature"`
	RiskLevel      string    `json:"riskLevel" bson:"risk_level"`
	Decision       string    `json:"decision" bson:"decision"`
}

func (r ModelResponse) Validate() error {
	if r.SchemaVersion != 1 {
		return fmt.Errorf("schema_version must be 1")
	}
	if strings.TrimSpace(r.EventID) == "" {
		return fmt.Errorf("event_id is required")
	}
	if strings.TrimSpace(r.TransactionID) == "" {
		return fmt.Errorf("transaction_id is required")
	}
	if r.RiskScore < 0 || r.RiskScore > 1 {
		return fmt.Errorf("risk_score must be between 0 and 1")
	}
	if r.PredictedLabel != 0 && r.PredictedLabel != 1 {
		return fmt.Errorf("predicted_label must be 0 or 1")
	}
	if r.Threshold < 0 || r.Threshold > 1 {
		return fmt.Errorf("threshold must be between 0 and 1")
	}
	expectedLabel := 0
	if r.RiskScore >= r.Threshold {
		expectedLabel = 1
	}
	if r.PredictedLabel != expectedLabel {
		return fmt.Errorf("predicted_label does not match risk_score and threshold")
	}
	if strings.TrimSpace(r.ModelVersion) == "" {
		return fmt.Errorf("model_version is required")
	}
	if strings.TrimSpace(r.FeatureVersion) == "" {
		return fmt.Errorf("feature_version is required")
	}
	if strings.TrimSpace(r.Signature) == "" {
		return fmt.Errorf("signature is required")
	}
	if r.OccurredAt.IsZero() || r.ScoredAt.IsZero() {
		return fmt.Errorf("occurred_at and scored_at are required")
	}
	return nil
}

// Assessment is the durable server-side representation used by the admin
// read model. ReceivedAt is assigned by the server and is not model time.
type Assessment struct {
	ID             string     `json:"assessmentId" bson:"_id"`
	SchemaVersion  int        `json:"schemaVersion" bson:"schema_version"`
	EventID        string     `json:"eventId" bson:"event_id"`
	TransactionID  string     `json:"transactionId" bson:"transaction_id"`
	OccurredAt     time.Time  `json:"occurredAt" bson:"occurred_at"`
	ScoredAt       time.Time  `json:"scoredAt" bson:"scored_at"`
	RiskScore      float64    `json:"riskScore" bson:"risk_score"`
	PredictedLabel int        `json:"predictedLabel" bson:"predicted_label"`
	Threshold      float64    `json:"threshold" bson:"threshold"`
	ModelVersion   string     `json:"modelVersion" bson:"model_version"`
	FeatureVersion string     `json:"featureVersion" bson:"feature_version"`
	Signature      string     `json:"-" bson:"signature"`
	RiskLevel      string     `json:"riskLevel" bson:"risk_level"`
	Decision       string     `json:"decision" bson:"decision"`
	Finalized      bool       `json:"-" bson:"finalized"`
	FinalizedAt    *time.Time `json:"-" bson:"finalized_at,omitempty"`
	ReceivedAt     time.Time  `json:"receivedAt" bson:"received_at"`
	KafkaTopic     string     `json:"-" bson:"kafka_topic"`
	KafkaPartition int        `json:"-" bson:"kafka_partition"`
	KafkaOffset    int64      `json:"-" bson:"kafka_offset"`
}

func NewAssessment(response ModelResponse, receivedAt time.Time, topic string, partition int, offset int64, id string) (Assessment, error) {
	if err := response.Validate(); err != nil {
		return Assessment{}, err
	}
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}
	// MongoDB DateTime has millisecond precision. Normalize before persisting
	// and publishing so cursor comparisons use the same value everywhere.
	receivedAt = receivedAt.UTC().Truncate(time.Millisecond)
	return Assessment{
		ID:             id,
		SchemaVersion:  response.SchemaVersion,
		EventID:        response.EventID,
		TransactionID:  response.TransactionID,
		OccurredAt:     response.OccurredAt.UTC(),
		ScoredAt:       response.ScoredAt.UTC(),
		RiskScore:      response.RiskScore,
		PredictedLabel: response.PredictedLabel,
		Threshold:      response.Threshold,
		ModelVersion:   response.ModelVersion,
		FeatureVersion: response.FeatureVersion,
		Signature:      response.Signature,
		RiskLevel:      LevelForScore(response.RiskScore),
		Decision:       DecisionForScore(response.RiskScore),
		ReceivedAt:     receivedAt,
		KafkaTopic:     topic,
		KafkaPartition: partition,
		KafkaOffset:    offset,
	}, nil
}

// CanonicalJSON returns the deterministic JSON payload that is authenticated.
// Go encoding/json sorts map keys, matching the model service's sorted-key,
// compact JSON canonicalization.
func CanonicalJSON(response ModelResponse) ([]byte, error) {
	return json.Marshal(map[string]any{
		"event_id":        response.EventID,
		"occurred_at":     response.OccurredAt,
		"predicted_label": response.PredictedLabel,
		"risk_score":      response.RiskScore,
		"schema_version":  response.SchemaVersion,
		"scored_at":       response.ScoredAt,
		"threshold":       response.Threshold,
		"transaction_id":  response.TransactionID,
		"model_version":   response.ModelVersion,
		"feature_version": response.FeatureVersion,
	})
}

func Sign(response ModelResponse, secretKey string) (string, error) {
	canonical, err := CanonicalJSON(response)
	if err != nil {
		return "", err
	}
	hasher := hmac.New(sha256.New, []byte(secretKey))
	_, _ = hasher.Write(canonical)
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func Verify(response ModelResponse, secretKey string) error {
	expected, err := Sign(response, secretKey)
	if err != nil {
		return err
	}
	provided := strings.ToLower(strings.TrimSpace(response.Signature))
	if !hmac.Equal([]byte(provided), []byte(expected)) {
		return fmt.Errorf("invalid model response signature")
	}
	return nil
}
