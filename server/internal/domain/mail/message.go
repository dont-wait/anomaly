package mail

import "time"

type MailMessage struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

// ReceiptOutboxItem is a durable transfer receipt waiting for delivery.
type ReceiptOutboxItem struct {
	ID            string     `bson:"_id"`
	To            string     `bson:"to"`
	Subject       string     `bson:"subject"`
	Text          string     `bson:"text"`
	HTML          string     `bson:"html"`
	Status        string     `bson:"status"`
	Attempts      int        `bson:"attempts"`
	NextAttemptAt time.Time  `bson:"next_attempt_at"`
	LockedAt      time.Time  `bson:"locked_at,omitempty"`
	LockID        string     `bson:"lock_id,omitempty"`
	LastError     string     `bson:"last_error,omitempty"`
	CreatedAt     time.Time  `bson:"created_at"`
	SentAt        *time.Time `bson:"sent_at,omitempty"`
}
