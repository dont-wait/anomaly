package timeline

import "time"

// LoanEvent is the masked, account-scoped representation of a loan event used
// by the admin account timeline. It deliberately does not expose the original
// loan event payload or document storage metadata.
type LoanEvent struct {
	EventID        string         `json:"eventId" bson:"_id"`
	EventType      string         `json:"eventType" bson:"event_type"`
	LoanID         string         `json:"loanId" bson:"loan_id"`
	AccountID      string         `json:"accountId" bson:"account_id"`
	SourceStream   string         `json:"sourceStream" bson:"source_stream"`
	SourceRevision uint64         `json:"sourceRevision" bson:"source_revision"`
	OccurredAt     time.Time      `json:"occurredAt" bson:"occurred_at"`
	Summary        string         `json:"summary" bson:"summary"`
	Details        map[string]any `json:"details,omitempty" bson:"details,omitempty"`
}
