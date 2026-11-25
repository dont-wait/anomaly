package riskassessment

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dont-wait/anomaly/internal/domain/risk"
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

// DecisionReasonForScore explains, for tracing, why DecisionForScore chose its
// result. It is derived on the server and is not part of the signed payload.
func DecisionReasonForScore(score float64) string {
	if score > RiskBlockScore {
		return fmt.Sprintf("risk_score %v > block threshold %v", score, RiskBlockScore)
	}
	return fmt.Sprintf("risk_score %v <= block threshold %v", score, RiskBlockScore)
}

// MaxTopFactors bounds the trace-only factor list accepted from the ML service.
const MaxTopFactors = 8

// Factor is a trace-only note about an unusual model input, in the wire format the
// ML service publishes (snake_case, like the rest of ModelResponse). Numeric
// features carry ZScore (standard deviations from the training mean); binary flags
// and rare fractions carry Value. It describes how atypical an input is, not how
// much it contributed to the score, and it is not covered by the signature, so it
// is advisory data that must not drive decisions.
type Factor struct {
	Feature string   `json:"feature" bson:"feature"`
	ZScore  *float64 `json:"z_score,omitempty" bson:"z_score,omitempty"`
	Value   *float64 `json:"value,omitempty" bson:"value,omitempty"`
	// Reason is a one-line human-readable explanation written by the ML service.
	Reason string `json:"reason,omitempty" bson:"reason,omitempty"`
}

// MaxFactorReasonLen bounds Reason, which is unsigned text from the ML service.
const MaxFactorReasonLen = 200

func (f Factor) validate() error {
	if strings.TrimSpace(f.Feature) == "" || len(f.Feature) > 64 {
		return fmt.Errorf("top_factors feature must be 1-64 characters")
	}
	if utf8.RuneCountInString(f.Reason) > MaxFactorReasonLen {
		return fmt.Errorf("top_factors reason accepts at most %d characters", MaxFactorReasonLen)
	}
	if (f.ZScore == nil) == (f.Value == nil) {
		return fmt.Errorf("top_factors %q needs exactly one of z_score and value", f.Feature)
	}
	for _, v := range []*float64{f.ZScore, f.Value} {
		if v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0)) {
			return fmt.Errorf("top_factors %q must be finite", f.Feature)
		}
	}
	return nil
}

// Scenario limits. Scenarios are unsigned text from the ML service, so they are
// bounded and shape-checked rather than trusted.
const (
	MaxScenarios            = 8
	MaxScenarioEvidence     = 8
	MaxScenarioTitleLen     = 80
	MaxScenarioReasonLen    = 300
	MaxScenarioEvidenceName = 64
)

var scenarioCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,47}$`)

// Scenario is a named fraud pattern matched by rules in the ML service (for
// example ACCOUNT_TAKEOVER), with a one-line reason quoting the numbers. It tells
// an investigator which pattern to look at. It is not model attribution, is not
// covered by the signature, and must not drive decisions. Code is a stable
// identifier for filtering and grouping; Evidence lists the model inputs involved.
type Scenario struct {
	Code     string   `json:"code" bson:"code"`
	Severity string   `json:"severity" bson:"severity"`
	Title    string   `json:"title" bson:"title"`
	Reason   string   `json:"reason" bson:"reason"`
	Evidence []string `json:"evidence" bson:"evidence"`
}

func (s Scenario) validate() error {
	if !scenarioCodePattern.MatchString(s.Code) {
		return fmt.Errorf("scenarios code %q must match %s", s.Code, scenarioCodePattern)
	}
	switch s.Severity {
	case "low", "medium", "high":
	default:
		return fmt.Errorf("scenarios %q severity must be low, medium or high", s.Code)
	}
	if t := strings.TrimSpace(s.Title); t == "" || utf8.RuneCountInString(s.Title) > MaxScenarioTitleLen {
		return fmt.Errorf("scenarios %q title must be 1-%d characters", s.Code, MaxScenarioTitleLen)
	}
	if r := strings.TrimSpace(s.Reason); r == "" || utf8.RuneCountInString(s.Reason) > MaxScenarioReasonLen {
		return fmt.Errorf("scenarios %q reason must be 1-%d characters", s.Code, MaxScenarioReasonLen)
	}
	if len(s.Evidence) > MaxScenarioEvidence {
		return fmt.Errorf("scenarios %q accepts at most %d evidence entries", s.Code, MaxScenarioEvidence)
	}
	for _, name := range s.Evidence {
		if strings.TrimSpace(name) == "" || len(name) > MaxScenarioEvidenceName {
			return fmt.Errorf("scenarios %q evidence must be 1-%d characters", s.Code, MaxScenarioEvidenceName)
		}
	}
	return nil
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
	// DecisionReason is derived by the server for tracing; never signed.
	DecisionReason string `json:"decisionReason,omitempty" bson:"decision_reason,omitempty"`
	// TopFactors is sent by the ML service for tracing. It is NOT covered by the
	// signature, so treat it as unverified, advisory data.
	TopFactors []Factor `json:"top_factors,omitempty" bson:"top_factors,omitempty"`
	// Scenarios is sent by the ML service for tracing. Like TopFactors it is NOT
	// covered by the signature, so treat it as unverified, advisory data.
	Scenarios []Scenario `json:"scenarios,omitempty" bson:"scenarios,omitempty"`
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
	if len(r.TopFactors) > MaxTopFactors {
		return fmt.Errorf("top_factors accepts at most %d entries", MaxTopFactors)
	}
	for _, factor := range r.TopFactors {
		if err := factor.validate(); err != nil {
			return err
		}
	}
	if len(r.Scenarios) > MaxScenarios {
		return fmt.Errorf("scenarios accepts at most %d entries", MaxScenarios)
	}
	for _, scenario := range r.Scenarios {
		if err := scenario.validate(); err != nil {
			return err
		}
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
	DecisionReason string     `json:"decisionReason,omitempty" bson:"decision_reason,omitempty"`
	TopFactors     []Factor   `json:"topFactors,omitempty" bson:"top_factors,omitempty"`
	Scenarios      []Scenario `json:"scenarios,omitempty" bson:"scenarios,omitempty"`
	Finalized      bool       `json:"-" bson:"finalized"`
	FinalizedAt    *time.Time `json:"-" bson:"finalized_at,omitempty"`
	ReceivedAt     time.Time  `json:"receivedAt" bson:"received_at"`
	Cursor         int64      `json:"-" bson:"cursor,omitempty"`
	KafkaTopic     string     `json:"-" bson:"kafka_topic"`
	KafkaPartition int        `json:"-" bson:"kafka_partition"`
	KafkaOffset    int64      `json:"-" bson:"kafka_offset"`
	Handled        bool       `json:"handled" bson:"handled,omitempty"`
	HandledAt      *time.Time `json:"handledAt,omitempty" bson:"handled_at,omitempty"`
	HandledBy      string     `json:"handledBy,omitempty" bson:"handled_by,omitempty"`
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
		DecisionReason: DecisionReasonForScore(response.RiskScore),
		TopFactors:     response.TopFactors,
		Scenarios:      response.Scenarios,
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
