package transaction

import (
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
)

const (
	Created             = "TransferCreated"
	Completed           = "TransferCompleted"
	Cancelled           = "TransferCancelled"
	OTPUpdated          = "TransferOTPUpdated"
	SchemaVersion int64 = 1
)

// Event is an immutable snapshot at a transition, shared by the canonical journal and downstream consumers.
// It intentionally excludes OTPs, identity details and fraud labels.
type Event struct {
	ID            string    `json:"eventId" bson:"event_id"`
	TransactionID string    `json:"transactionId" bson:"transaction_id"`
	Type          string    `json:"eventType" bson:"event_type"`
	SchemaVersion int64     `json:"schemaVersion" bson:"schema_version"`
	Sequence      int64     `json:"sequence" bson:"sequence"`
	OccurredAt    time.Time `json:"occurredAt" bson:"occurred_at"`
	Payload       Payload   `json:"payload" bson:"payload"`
}

type Payload struct {
	SourceAccountID          string `json:"sourceAccountId" bson:"source_account_id"`
	DestinationAccountID     string `json:"destinationAccountId" bson:"destination_account_id"`
	Amount                   int64  `json:"amount" bson:"amount"`
	Fee                      int64  `json:"fee" bson:"fee"`
	Currency                 string `json:"currency" bson:"currency"`
	Channel                  string `json:"channel" bson:"channel"`
	Status                   string `json:"status" bson:"status"`
	SourceBalanceBefore      *int64 `json:"sourceBalanceBefore,omitempty" bson:"source_balance_before,omitempty"`
	SourceBalanceAfter       *int64 `json:"sourceBalanceAfter,omitempty" bson:"source_balance_after,omitempty"`
	DestinationBalanceBefore *int64 `json:"destinationBalanceBefore,omitempty" bson:"destination_balance_before,omitempty"`
	DestinationBalanceAfter  *int64 `json:"destinationBalanceAfter,omitempty" bson:"destination_balance_after,omitempty"`
}

// Stable IDs make database retries and delivery retries refer to the same event.
func EventID(transactionID, eventType string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(transactionID+":"+eventType)).String()
}

func IsTransferEvent(eventType string) bool {
	return eventType == Created || eventType == Completed || eventType == Cancelled || eventType == OTPUpdated
}

func (e Event) Validate() error {
	if _, err := uuid.Parse(e.ID); err != nil {
		return fmt.Errorf("invalid event id: %w", err)
	}
	if _, err := uuid.Parse(e.TransactionID); err != nil {
		return fmt.Errorf("invalid transaction id: %w", err)
	}
	if e.SchemaVersion != SchemaVersion || !IsTransferEvent(e.Type) || e.OccurredAt.IsZero() {
		return fmt.Errorf("unsupported or incomplete transfer event")
	}
	wantSequence, wantStatus := int64(2), "cancelled"
	if e.Type == Created {
		wantSequence, wantStatus = 1, "awaiting_otp"
	}
	if e.Type == Completed {
		wantStatus = "success"
	}
	if e.Type == OTPUpdated {
		wantStatus = "awaiting_otp"
	}
	p := e.Payload
	if (e.Type == Created && e.Sequence != wantSequence) || (e.Type != Created && e.Sequence < 2) || p.Status != wantStatus || p.Amount <= 0 || p.Fee < 0 || p.Currency != "VND" || p.SourceAccountID == "" || p.DestinationAccountID == "" || p.SourceAccountID == p.DestinationAccountID {
		return fmt.Errorf("invalid transfer event payload or sequence")
	}
	if p.Channel != "web" && p.Channel != "mobile" && p.Channel != "desktop" {
		return fmt.Errorf("invalid channel")
	}
	if e.Type == Completed && (p.SourceBalanceBefore == nil || p.SourceBalanceAfter == nil || p.DestinationBalanceBefore == nil || p.DestinationBalanceAfter == nil) {
		return fmt.Errorf("completed transfer has no balance snapshots")
	}
	if e.Type == Completed {
		sourceBefore, sourceAfter := *p.SourceBalanceBefore, *p.SourceBalanceAfter
		destinationBefore, destinationAfter := *p.DestinationBalanceBefore, *p.DestinationBalanceAfter
		if p.Amount > math.MaxInt64-p.Fee || sourceBefore < p.Amount+p.Fee || sourceAfter != sourceBefore-p.Amount-p.Fee || destinationBefore < 0 || destinationBefore > math.MaxInt64-p.Amount || destinationAfter != destinationBefore+p.Amount {
			return fmt.Errorf("inconsistent completed transfer balances")
		}
	}
	return nil
}
