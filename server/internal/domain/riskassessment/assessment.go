package riskassessment

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// ModelResponse is the scoring contract emitted by the ML service. It is an
// assessment only; predicted_label must not be treated as an ALLOW/BLOCK
// business decision.
type ModelResponse struct {
	TransactionID  string  `json:"transaction_id" bson:"transaction_id"`
	RiskScore      float64 `json:"risk_score" bson:"risk_score"`
	PredictedLabel int     `json:"predicted_label" bson:"predicted_label"`
	Threshold      float64 `json:"threshold" bson:"threshold"`
	ModelVersion   string  `json:"model_version" bson:"model_version"`
	FeatureVersion string  `json:"feature_version" bson:"feature_version"`
}

func (r ModelResponse) Validate() error {
	if strings.TrimSpace(r.TransactionID) == "" {
		return fmt.Errorf("transaction_id is required")
	}
	if math.IsNaN(r.RiskScore) || math.IsInf(r.RiskScore, 0) || r.RiskScore < 0 || r.RiskScore > 1 {
		return fmt.Errorf("risk_score must be between 0 and 1")
	}
	if r.PredictedLabel != 0 && r.PredictedLabel != 1 {
		return fmt.Errorf("predicted_label must be 0 or 1")
	}
	if math.IsNaN(r.Threshold) || math.IsInf(r.Threshold, 0) || r.Threshold < 0 || r.Threshold > 1 {
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
	return nil
}

// Assessment is the durable server-side representation used by the admin
// read model. ReceivedAt is assigned by the server and is not model time.
type Assessment struct {
	ID             string    `json:"assessmentId" bson:"_id"`
	TransactionID  string    `json:"transactionId" bson:"transaction_id"`
	RiskScore      float64   `json:"riskScore" bson:"risk_score"`
	PredictedLabel int       `json:"predictedLabel" bson:"predicted_label"`
	Threshold      float64   `json:"threshold" bson:"threshold"`
	ModelVersion   string    `json:"modelVersion" bson:"model_version"`
	FeatureVersion string    `json:"featureVersion" bson:"feature_version"`
	ReceivedAt     time.Time `json:"receivedAt" bson:"received_at"`
	KafkaTopic     string    `json:"-" bson:"kafka_topic"`
	KafkaPartition int       `json:"-" bson:"kafka_partition"`
	KafkaOffset    int64     `json:"-" bson:"kafka_offset"`
}

func NewAssessment(response ModelResponse, receivedAt time.Time, topic string, partition int, offset int64, id string) (Assessment, error) {
	if err := response.Validate(); err != nil {
		return Assessment{}, err
	}
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}
	return Assessment{
		ID:             id,
		TransactionID:  response.TransactionID,
		RiskScore:      response.RiskScore,
		PredictedLabel: response.PredictedLabel,
		Threshold:      response.Threshold,
		ModelVersion:   response.ModelVersion,
		FeatureVersion: response.FeatureVersion,
		ReceivedAt:     receivedAt.UTC(),
		KafkaTopic:     topic,
		KafkaPartition: partition,
		KafkaOffset:    offset,
	}, nil
}
