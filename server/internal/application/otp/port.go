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
}

// MailSender gửi mail. Implementation cụ thể nằm ở infrastructure
// (stdlib net/smtp); dùng interface để test fake dễ.
type MailSender interface {
	Send(ctx context.Context, msg maildomain.MailMessage) error
}
