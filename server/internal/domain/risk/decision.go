package risk

import (
	"fmt"
	"strings"
	"time"
)

const (
	DecisionPending = "PENDING"
	DecisionAllow   = "ALLOW"
	DecisionBlock   = "BLOCK"
)

type Assessment struct {
	Decision     string    `json:"decision,omitempty" bson:"decision,omitempty"`
	IsAnomalous  bool      `json:"is_anomalous" bson:"is_anomalous"`
	RiskScore    float64   `json:"risk_score" bson:"risk_score"`
	ModelVersion string    `json:"model_version" bson:"model_version"`
	EvaluatedAt  time.Time `json:"evaluated_at" bson:"evaluated_at"`
}

func (a Assessment) Validate() error {
	if a.RiskScore < 0 || a.RiskScore > 1 {
		return fmt.Errorf("risk score must be between 0 and 1")
	}
	if strings.TrimSpace(a.ModelVersion) == "" || a.EvaluatedAt.IsZero() {
		return fmt.Errorf("risk assessment metadata is required")
	}
	return nil
}

type Decision struct {
	TransferID    string     `json:"transfer_id" bson:"transfer_id"`
	SourceEventID string     `json:"source_event_id" bson:"source_event_id"`
	Decision      string     `json:"decision" bson:"decision"`
	Assessment    Assessment `json:"assessment" bson:"assessment"`
	CreatedAt     time.Time  `json:"created_at" bson:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" bson:"updated_at"`
	ExpiresAt     time.Time  `json:"expires_at" bson:"expires_at"`
	Finalized     bool       `json:"finalized" bson:"finalized"`
	FinalizedAt   *time.Time `json:"finalized_at,omitempty" bson:"finalized_at,omitempty"`
}

func (d Decision) Validate() error {
	if strings.TrimSpace(d.TransferID) == "" {
		return fmt.Errorf("transfer id is required")
	}
	if strings.TrimSpace(d.SourceEventID) == "" {
		return fmt.Errorf("source event id is required")
	}
	if d.Decision != DecisionPending && d.Decision != DecisionAllow && d.Decision != DecisionBlock {
		return fmt.Errorf("unsupported risk decision %q", d.Decision)
	}
	if d.Decision == DecisionPending {
		return nil
	}
	if d.Assessment.Decision != "" && d.Assessment.Decision != d.Decision {
		return fmt.Errorf("risk assessment decision does not match decision")
	}
	if err := d.Assessment.Validate(); err != nil {
		return err
	}
	if d.Decision == DecisionBlock && !d.Assessment.IsAnomalous {
		return fmt.Errorf("blocked decision must be anomalous")
	}
	if d.Decision == DecisionAllow && d.Assessment.IsAnomalous {
		return fmt.Errorf("allowed decision cannot be anomalous")
	}
	return nil
}
