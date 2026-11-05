// Package notification models user-facing notifications. A notification is a
// read model for display only: it never decides funds or command state.
package notification

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

type (
	Audience string
	Category string
	Severity string
	Type     string
)

const (
	AudienceCustomer Audience = "CUSTOMER"
	AudienceAdmin    Audience = "ADMIN"
)

const (
	CategoryTransaction Category = "TRANSACTION"
	CategorySecurity    Category = "SECURITY"
	CategoryAccount     Category = "ACCOUNT"
	CategoryLoan        Category = "LOAN"
	CategorySavings     Category = "SAVINGS"
	CategorySystem      Category = "SYSTEM"
)

const (
	SeverityInfo     Severity = "INFO"
	SeveritySuccess  Severity = "SUCCESS"
	SeverityWarning  Severity = "WARNING"
	SeverityCritical Severity = "CRITICAL"
)

const (
	TypeTransferSent       Type = "TRANSFER_SENT"
	TypeTransferReceived   Type = "TRANSFER_RECEIVED"
	TypeTransferBlocked    Type = "TRANSFER_BLOCKED"
	TypeSuspiciousActivity Type = "SUSPICIOUS_ACTIVITY"

	TypeAccountLocked    Type = "ACCOUNT_LOCKED"
	TypeAccountUnlocked  Type = "ACCOUNT_UNLOCKED"
	TypeLoanApproved     Type = "LOAN_APPROVED"
	TypeLoanDisbursed    Type = "LOAN_DISBURSED"
	TypeLoanRepaymentDue Type = "LOAN_REPAYMENT_DUE"
	TypeSavingsMatured   Type = "SAVINGS_MATURED"

	// TypeSystemTest is only triggered by the development seeder.
	TypeSystemTest Type = "SYSTEM_TEST"
)

const (
	MaxDataKeys       = 20
	MaxDataValueRunes = 256
)

var (
	ErrNotFound = errors.New("notification not found")
	ErrInvalid  = errors.New("invalid notification")
)

type Notification struct {
	ID            string
	Audience      Audience
	RecipientID   string
	Type          Type
	Category      Category
	Severity      Severity
	Title         string
	Body          string
	Data          map[string]string
	SourceEventID string
	OccurredAt    time.Time
	CreatedAt     time.Time
	ReadAt        *time.Time
	ExpiresAt     time.Time
}

func (a Audience) Valid() bool { return a == AudienceCustomer || a == AudienceAdmin }

func (s Severity) Valid() bool {
	switch s {
	case SeverityInfo, SeveritySuccess, SeverityWarning, SeverityCritical:
		return true
	}
	return false
}

func (c Category) Valid() bool {
	switch c {
	case CategoryTransaction, CategorySecurity, CategoryAccount, CategoryLoan, CategorySavings, CategorySystem:
		return true
	}
	return false
}

// RecipientKey scopes a recipient id to its audience so customer and staff ids
// can never collide in the hub or the store.
func RecipientKey(audience Audience, recipientID string) string {
	return string(audience) + ":" + recipientID
}

// NotificationID is deterministic so replays, retries and duplicate workers
// converge on one document. Same pattern as transaction.EventID.
func NotificationID(sourceEventID string, t Type, recipientID string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(sourceEventID+":"+string(t)+":"+recipientID)).String()
}

func (n Notification) Validate() error {
	switch {
	case strings.TrimSpace(n.ID) == "":
		return fmt.Errorf("%w: id is required", ErrInvalid)
	case !n.Audience.Valid():
		return fmt.Errorf("%w: unsupported audience %q", ErrInvalid, n.Audience)
	case strings.TrimSpace(n.RecipientID) == "":
		return fmt.Errorf("%w: recipient id is required", ErrInvalid)
	case strings.TrimSpace(string(n.Type)) == "":
		return fmt.Errorf("%w: type is required", ErrInvalid)
	case !n.Category.Valid():
		return fmt.Errorf("%w: unsupported category %q", ErrInvalid, n.Category)
	case !n.Severity.Valid():
		return fmt.Errorf("%w: unsupported severity %q", ErrInvalid, n.Severity)
	case strings.TrimSpace(n.SourceEventID) == "":
		return fmt.Errorf("%w: source event id is required", ErrInvalid)
	case strings.TrimSpace(n.Title) == "":
		return fmt.Errorf("%w: title is required", ErrInvalid)
	case n.OccurredAt.IsZero() || n.CreatedAt.IsZero() || n.ExpiresAt.IsZero():
		return fmt.Errorf("%w: occurredAt, createdAt and expiresAt are required", ErrInvalid)
	case len(n.Data) > MaxDataKeys:
		return fmt.Errorf("%w: data has %d keys, max %d", ErrInvalid, len(n.Data), MaxDataKeys)
	}
	for key, value := range n.Data {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("%w: data key is empty", ErrInvalid)
		}
		if utf8.RuneCountInString(value) > MaxDataValueRunes {
			return fmt.Errorf("%w: data %q exceeds %d characters", ErrInvalid, key, MaxDataValueRunes)
		}
	}
	return nil
}
