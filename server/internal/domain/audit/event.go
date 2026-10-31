package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	TransferOTPVerificationSucceeded = "TransferOTPVerificationSucceeded"
	TransferOTPVerificationFailed    = "TransferOTPVerificationFailed"
	TransferOTPExpired               = "TransferOTPExpired"
	TransferOTPAttemptsExceeded      = "TransferOTPAttemptsExceeded"

	EmailOTPVerificationSucceeded = "EmailOTPVerificationSucceeded"
	EmailOTPVerificationFailed    = "EmailOTPVerificationFailed"
	EmailOTPExpired               = "EmailOTPExpired"
	EmailOTPAttemptsExceeded      = "EmailOTPAttemptsExceeded"

	LoginSucceeded = "LoginSucceeded"
	LoginFailed    = "LoginFailed"
)

// Event is an immutable, non-financial audit record. Secrets are deliberately
// not represented by this type: OTP values, passwords and tokens must never be
// written to EventStoreDB.
type Event struct {
	ID            string    `json:"id"`
	Type          string    `json:"type"`
	SchemaVersion int64     `json:"schemaVersion"`
	OccurredAt    time.Time `json:"occurredAt"`
	OperationID   string    `json:"operationId,omitempty"`
	CorrelationID string    `json:"correlationId,omitempty"`
	AccountID     string    `json:"accountId,omitempty"`
	ActorID       string    `json:"actorId,omitempty"`
	TransactionID string    `json:"transactionId,omitempty"`
	ChallengeID   string    `json:"challengeId,omitempty"`
	ReasonCode    string    `json:"reasonCode,omitempty"`
	AttemptsLeft  *int      `json:"attemptsLeft,omitempty"`
	EmailHash     string    `json:"emailHash,omitempty"`
}

// Recorder is intentionally small so application packages do not depend on
// the EventStoreDB client or its infrastructure package.
type Recorder interface {
	Record(context.Context, Event) error
}

func NewEvent(eventType, operationID string, now time.Time) Event {
	if operationID == "" {
		operationID = uuid.NewString()
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return Event{
		ID:            stableID(eventType, operationID),
		Type:          eventType,
		SchemaVersion: 1,
		OccurredAt:    now.UTC(),
		OperationID:   operationID,
		CorrelationID: operationID,
	}
}

// HashEmail provides a stable correlation value for pre-account email OTP
// events without storing the email address in the audit payload.
func HashEmail(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(sum[:])
}

func stableID(eventType, operationID string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(eventType+":"+operationID)).String()
}
