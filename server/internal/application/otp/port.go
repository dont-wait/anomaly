package otp

import (
	"context"
	"time"

	maildomain "github.com/dont-wait/anomaly/internal/domain/mail"
)

// OTPStore lưu mã OTP theo email. Implementation cụ thể nằm ở
// infrastructure (Redis); miss khi Get được map về ErrOTPExpired.
type OTPStore interface {
	Set(ctx context.Context, email, code string, ttl time.Duration) error
	Get(ctx context.Context, email string) (string, error)
	Del(ctx context.Context, email string) error

	// Consume atomically compares code with stored value and deletes key
	// on match. Returns (true, nil) on success, (false, nil) on mismatch,
	// (false, ErrOTPExpired) when key is missing.
	Consume(ctx context.Context, email, code string) (bool, error)

	// DelIfMatch deletes the key only if stored code matches.
	// Best-effort cleanup for failed sends; returns nil even when
	// key is missing or code doesn't match (only Redis errors matter).
	DelIfMatch(ctx context.Context, email, code string) error

	// SetCooldown sets a key with TTL if not already present.
	// Returns true if the key was set (request allowed), false if
	// the key already exists (cooldown active).
	SetCooldown(ctx context.Context, key string, ttl time.Duration) (bool, error)

	// IncrAttempts atomically increments a counter key and sets TTL
	// on first increment. Returns the new counter value.
	IncrAttempts(ctx context.Context, key string, ttl time.Duration) (int64, error)
}

// MailSender gửi mail. Implementation cụ thể nằm ở infrastructure
// (stdlib net/smtp); dùng interface để test fake dễ.
type MailSender interface {
	Send(ctx context.Context, msg maildomain.MailMessage) error
}
