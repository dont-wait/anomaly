package otp

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	maildomain "github.com/dont-wait/anomaly/internal/domain/mail"
	otpdomain "github.com/dont-wait/anomaly/internal/domain/otp"
	"github.com/redis/go-redis/v9"
)

var (
	ErrTransferOTPExpired  = errors.New("otp expired")
	ErrTransferOTPInvalid  = errors.New("invalid otp")
	ErrTransferOTPExceeded = errors.New("otp attempts exceeded")
	ErrTransferOTPTooSoon  = errors.New("otp resend too soon")
)

const (
	transferOTPTTL      = 5 * time.Minute
	transferOTPCooldown = 30 * time.Second
	transferOTPAttempts = 3
)

type TransferMailSender interface {
	Send(context.Context, maildomain.MailMessage) error
}

type TransferOTPInfo struct {
	Channel           string    `json:"channel"`
	MaskedDestination string    `json:"maskedDestination"`
	ExpiresAt         time.Time `json:"expiresAt"`
	AttemptsLeft      int       `json:"attemptsLeft"`
	ResendAvailableAt time.Time `json:"resendAvailableAt"`
}

type TransferOTPStore struct {
	rdb  *redis.Client
	mail TransferMailSender
}

func NewTransferOTPStore(rdb *redis.Client, sender TransferMailSender) *TransferOTPStore {
	return &TransferOTPStore{rdb: rdb, mail: sender}
}

func (s *TransferOTPStore) Issue(ctx context.Context, transferID, email string) (TransferOTPInfo, error) {
	return s.issue(ctx, transferID, email)
}

func (s *TransferOTPStore) Resend(ctx context.Context, transferID, email string) (TransferOTPInfo, error) {
	return s.issue(ctx, transferID, email)
}

func (s *TransferOTPStore) issue(ctx context.Context, transferID, email string) (TransferOTPInfo, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	code, err := otpdomain.GenerateCode()
	if err != nil {
		return TransferOTPInfo{}, err
	}
	key := transferOTPKey(transferID)
	cooldownKey := transferOTPCooldownKey(email)
	ok, err := s.rdb.SetNX(ctx, cooldownKey, "1", transferOTPCooldown).Result()
	if err != nil {
		return TransferOTPInfo{}, err
	}
	if !ok {
		return TransferOTPInfo{}, ErrTransferOTPTooSoon
	}
	if err := s.rdb.Set(ctx, key, code, transferOTPTTL).Err(); err != nil {
		_ = s.rdb.Del(ctx, cooldownKey).Err()
		return TransferOTPInfo{}, err
	}
	text := fmt.Sprintf("Mã OTP xác nhận chuyển tiền AnomalyBank của bạn là %s. Mã có hiệu lực trong 5 phút. Không chia sẻ mã này với bất kỳ ai.", code)
	html := fmt.Sprintf("<p>Mã OTP xác nhận chuyển tiền AnomalyBank của bạn:</p><p style=\"font-size:24px;font-weight:bold;letter-spacing:4px\">%s</p><p>Mã có hiệu lực trong 5 phút. Không chia sẻ mã này với bất kỳ ai.</p>", code)
	if err := s.mail.Send(ctx, maildomain.MailMessage{To: email, Subject: "OTP xác nhận chuyển tiền AnomalyBank", Text: text, HTML: html}); err != nil {
		_ = s.rdb.Del(ctx, key).Err()
		_ = s.rdb.Del(ctx, cooldownKey).Err()
		return TransferOTPInfo{}, err
	}
	_ = s.rdb.Del(ctx, transferOTPAttemptsKey(transferID)).Err()
	now := time.Now().UTC()
	return TransferOTPInfo{Channel: "email", MaskedDestination: maskEmail(email), ExpiresAt: now.Add(transferOTPTTL), AttemptsLeft: transferOTPAttempts, ResendAvailableAt: now.Add(transferOTPCooldown)}, nil
}

// Verify does not consume a valid code; the caller consumes it only after the
// MongoDB transaction commits, so transient database errors remain retryable.
func (s *TransferOTPStore) Verify(ctx context.Context, transferID, code string) (int, error) {
	key := transferOTPKey(transferID)
	stored, err := s.rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return 0, ErrTransferOTPExpired
	}
	if err != nil {
		return 0, err
	}
	if err := otpdomain.ValidateCode(code); err != nil {
		return s.recordFailure(ctx, transferID)
	}
	if subtle.ConstantTimeCompare([]byte(stored), []byte(code)) == 1 {
		return transferOTPAttempts, nil
	}
	return s.recordFailure(ctx, transferID)
}

func (s *TransferOTPStore) recordFailure(ctx context.Context, transferID string) (int, error) {
	count, err := s.rdb.Incr(ctx, transferOTPAttemptsKey(transferID)).Result()
	if err != nil {
		return 0, err
	}
	if count == 1 {
		_ = s.rdb.Expire(ctx, transferOTPAttemptsKey(transferID), transferOTPTTL).Err()
	}
	left := transferOTPAttempts - int(count)
	if left <= 0 {
		_ = s.rdb.Del(ctx, transferOTPKey(transferID), transferOTPAttemptsKey(transferID)).Err()
		return 0, ErrTransferOTPExceeded
	}
	return left, ErrTransferOTPInvalid
}

func (s *TransferOTPStore) Consume(ctx context.Context, transferID string) error {
	return s.rdb.Del(ctx, transferOTPKey(transferID), transferOTPAttemptsKey(transferID)).Err()
}

func (s *TransferOTPStore) Delete(ctx context.Context, transferID string) error {
	return s.Consume(ctx, transferID)
}

func transferOTPKey(id string) string         { return "transfer:otp:" + id }
func transferOTPAttemptsKey(id string) string { return "transfer:otp:attempts:" + id }
func transferOTPCooldownKey(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return "transfer:otp:cooldown:" + hex.EncodeToString(sum[:])
}

func maskEmail(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return "***"
	}
	local := parts[0]
	if len(local) <= 2 {
		return local[:1] + "***@" + parts[1]
	}
	return local[:2] + "***@" + parts[1]
}
