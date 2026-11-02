package otp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	appotp "github.com/dont-wait/anomaly/internal/application/otp"
	maildomain "github.com/dont-wait/anomaly/internal/domain/mail"
)

// EmailChangeTTL keeps the pending address alive exactly as long as its OTPs.
const EmailChangeTTL = transferOTPTTL

// NewEmailChangeOTPStore sends the challenge that proves control of the new address.
func NewEmailChangeOTPStore(rdb *redis.Client, sender TransferMailSender) *TransferOTPStore {
	return &TransferOTPStore{rdb: rdb, mail: sender, prefix: "email-change", purpose: appotp.OTPEmailChange}
}

// NewPasswordResetOTPStore sends the challenge that proves control of the account email.
func NewPasswordResetOTPStore(rdb *redis.Client, sender TransferMailSender) *TransferOTPStore {
	return &TransferOTPStore{rdb: rdb, mail: sender, prefix: "password-reset", purpose: appotp.OTPEmailPasswordReset}
}

type PendingEmailStore struct{ rdb *redis.Client }

func NewPendingEmailStore(rdb *redis.Client) *PendingEmailStore {
	return &PendingEmailStore{rdb: rdb}
}

func pendingEmailKey(accountID string) string { return "email-change:pending:" + accountID }

func (s *PendingEmailStore) Set(ctx context.Context, accountID, email string, ttl time.Duration) error {
	return s.rdb.Set(ctx, pendingEmailKey(accountID), email, ttl).Err()
}

func (s *PendingEmailStore) Get(ctx context.Context, accountID string) (string, error) {
	email, err := s.rdb.Get(ctx, pendingEmailKey(accountID)).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	return email, err
}

func (s *PendingEmailStore) Delete(ctx context.Context, accountID string) error {
	return s.rdb.Del(ctx, pendingEmailKey(accountID)).Err()
}

type EmailChangeNotifier struct {
	mail   TransferMailSender
	logger zerolog.Logger
}

func NewEmailChangeNotifier(sender TransferMailSender, logger zerolog.Logger) *EmailChangeNotifier {
	return &EmailChangeNotifier{mail: sender, logger: logger}
}

func (n *EmailChangeNotifier) NotifyEmailChanged(ctx context.Context, previous, next string) {
	text := fmt.Sprintf("Email tài khoản AnomalyBank của bạn đã được đổi sang %s. Nếu không phải bạn thực hiện, hãy liên hệ ngân hàng ngay.\n\nYour AnomalyBank account email was changed to %s. If this was not you, contact the bank immediately.", maskEmail(next), maskEmail(next))
	err := n.mail.Send(ctx, maildomain.MailMessage{To: previous, Subject: "[AnomalyBank] Email tài khoản đã thay đổi / Account email changed", Text: text, HTML: "<p>" + text + "</p>"})
	if err != nil {
		n.logger.Warn().Err(err).Msg("send email change notice failed")
	}
}

type PasswordResetNotifier struct {
	mail   TransferMailSender
	logger zerolog.Logger
}

func NewPasswordResetNotifier(sender TransferMailSender, logger zerolog.Logger) *PasswordResetNotifier {
	return &PasswordResetNotifier{mail: sender, logger: logger}
}

func (n *PasswordResetNotifier) NotifyPasswordReset(ctx context.Context, email string) {
	text := "Mật khẩu tài khoản AnomalyBank của bạn vừa được đặt lại. Nếu không phải bạn thực hiện, hãy liên hệ ngân hàng ngay.\n\nYour AnomalyBank password was just reset. If this was not you, contact the bank immediately."
	err := n.mail.Send(ctx, maildomain.MailMessage{To: email, Subject: "[AnomalyBank] Mật khẩu đã được đặt lại / Password reset", Text: text, HTML: "<p>" + text + "</p>"})
	if err != nil {
		n.logger.Warn().Err(err).Msg("send password reset notice failed")
	}
}

// PasswordAttemptLimiter counts failed current-password checks in Redis. Redis
// errors propagate so a password check fails closed instead of going unthrottled.
type PasswordAttemptLimiter struct {
	rdb          *redis.Client
	accountLimit int64
	sourceLimit  int64
	window       time.Duration
	prefix       string
}

func NewPasswordAttemptLimiter(rdb *redis.Client, accountLimit, sourceLimit int, window time.Duration) *PasswordAttemptLimiter {
	return &PasswordAttemptLimiter{rdb: rdb, accountLimit: int64(accountLimit), sourceLimit: int64(sourceLimit), window: window, prefix: "password-attempts"}
}

// NewPasswordResetAttemptLimiter counts separately, so unauthenticated reset
// guesses cannot lock a signed-in customer out of changing their password.
func NewPasswordResetAttemptLimiter(rdb *redis.Client, accountLimit, sourceLimit int, window time.Duration) *PasswordAttemptLimiter {
	limiter := NewPasswordAttemptLimiter(rdb, accountLimit, sourceLimit, window)
	limiter.prefix = "password-reset-attempts"
	return limiter
}

func (l *PasswordAttemptLimiter) accountKey(accountID string) string {
	return l.prefix + ":account:" + accountID
}

func (l *PasswordAttemptLimiter) sourceKey(source string) string {
	return l.prefix + ":source:" + source
}

func (l *PasswordAttemptLimiter) Allow(ctx context.Context, accountID, source string) (bool, error) {
	accountCount, err := l.count(ctx, l.accountKey(accountID))
	if err != nil {
		return false, err
	}
	sourceCount, err := l.count(ctx, l.sourceKey(source))
	if err != nil {
		return false, err
	}
	return accountCount < l.accountLimit && sourceCount < l.sourceLimit, nil
}

func (l *PasswordAttemptLimiter) count(ctx context.Context, key string) (int64, error) {
	count, err := l.rdb.Get(ctx, key).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return count, err
}

func (l *PasswordAttemptLimiter) RecordFailure(ctx context.Context, accountID, source string) error {
	for _, key := range []string{l.accountKey(accountID), l.sourceKey(source)} {
		count, err := l.rdb.Incr(ctx, key).Result()
		if err != nil {
			return err
		}
		if count == 1 {
			if err := l.rdb.Expire(ctx, key, l.window).Err(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (l *PasswordAttemptLimiter) Reset(ctx context.Context, accountID string) error {
	return l.rdb.Del(ctx, l.accountKey(accountID)).Err()
}
