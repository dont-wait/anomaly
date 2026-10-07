package otp

import (
	"context"
	"crypto/sha256"
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

// transferOTPIssueScript installs a new challenge and clears state from any
// previous challenge for the transfer.
var transferOTPIssueScript = redis.NewScript(`
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
redis.call('DEL', KEYS[2], KEYS[3])
return 1
`)

// transferOTPVerifyScript atomically moves a valid challenge to a retryable
// authorization, or records a failed attempt bounded by the challenge TTL.
// Returns {status, attempts left}: expired=0, invalid=1, verified=2, exceeded=3.
var transferOTPVerifyScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[3]) == 1 then
	return {2, tonumber(ARGV[2])}
end

local stored = redis.call('GET', KEYS[1])
if not stored then
	return {0, 0}
end

local ttl = redis.call('PTTL', KEYS[1])
if ttl <= 0 then
	redis.call('DEL', KEYS[1], KEYS[2])
	return {0, 0}
end

if stored == ARGV[1] then
	redis.call('SET', KEYS[3], '1', 'PX', ttl)
	redis.call('DEL', KEYS[1], KEYS[2])
	return {2, tonumber(ARGV[2])}
end

local attempts = redis.call('INCR', KEYS[2])
if attempts >= tonumber(ARGV[2]) then
	redis.call('DEL', KEYS[1], KEYS[2])
	return {3, 0}
end
redis.call('PEXPIRE', KEYS[2], ttl)
return {1, tonumber(ARGV[2]) - attempts}
`)

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
	if err := transferOTPIssueScript.Run(ctx, s.rdb, []string{
		key,
		transferOTPAttemptsKey(transferID),
		transferOTPVerifiedKey(transferID),
	}, code, transferOTPTTL.Milliseconds()).Err(); err != nil {
		_ = s.rdb.Del(ctx, cooldownKey).Err()
		return TransferOTPInfo{}, err
	}
	text := fmt.Sprintf("Mã OTP xác nhận chuyển tiền AnomalyBank của bạn là %s. Mã có hiệu lực trong 5 phút. Không chia sẻ mã này với bất kỳ ai.", code)
	html := fmt.Sprintf("<p>Mã OTP xác nhận chuyển tiền AnomalyBank của bạn:</p><p style=\"font-size:24px;font-weight:bold;letter-spacing:4px\">%s</p><p>Mã có hiệu lực trong 5 phút. Không chia sẻ mã này với bất kỳ ai.</p>", code)
	if err := s.mail.Send(ctx, maildomain.MailMessage{To: email, Subject: "OTP xác nhận chuyển tiền AnomalyBank", Text: text, HTML: html}); err != nil {
		_ = s.Consume(ctx, transferID)
		_ = s.rdb.Del(ctx, cooldownKey).Err()
		return TransferOTPInfo{}, err
	}
	now := time.Now().UTC()
	return TransferOTPInfo{Channel: "email", MaskedDestination: maskEmail(email), ExpiresAt: now.Add(transferOTPTTL), AttemptsLeft: transferOTPAttempts, ResendAvailableAt: now.Add(transferOTPCooldown)}, nil
}

// Verify replaces a valid challenge with an authorization that remains usable
// until the caller consumes it after the canonical EventStore append commits.
func (s *TransferOTPStore) Verify(ctx context.Context, transferID, code string) (int, error) {
	// Invalid code syntax is deliberately sent through the same atomic failure
	// path so it consumes an attempt exactly like any other wrong code.
	if err := otpdomain.ValidateCode(code); err != nil {
		code = ""
	}
	result, err := transferOTPVerifyScript.Run(ctx, s.rdb, []string{
		transferOTPKey(transferID),
		transferOTPAttemptsKey(transferID),
		transferOTPVerifiedKey(transferID),
	}, code, transferOTPAttempts).Int64Slice()
	if err != nil {
		return 0, err
	}
	if len(result) != 2 {
		return 0, fmt.Errorf("unexpected transfer OTP verification result length: %d", len(result))
	}
	left := int(result[1])
	switch result[0] {
	case 0:
		return 0, ErrTransferOTPExpired
	case 1:
		return left, ErrTransferOTPInvalid
	case 2:
		return left, nil
	case 3:
		return 0, ErrTransferOTPExceeded
	default:
		return 0, fmt.Errorf("unexpected transfer OTP verification result: %d", result[0])
	}
}

func (s *TransferOTPStore) Consume(ctx context.Context, transferID string) error {
	return s.rdb.Del(ctx,
		transferOTPKey(transferID),
		transferOTPAttemptsKey(transferID),
		transferOTPVerifiedKey(transferID),
	).Err()
}

func (s *TransferOTPStore) Delete(ctx context.Context, transferID string) error {
	return s.Consume(ctx, transferID)
}

func transferOTPKey(id string) string         { return "transfer:otp:" + id }
func transferOTPAttemptsKey(id string) string { return "transfer:otp:attempts:" + id }
func transferOTPVerifiedKey(id string) string { return "transfer:otp:verified:" + id }
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
