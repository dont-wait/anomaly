package otp

import (
	"context"
	"errors"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/audit"
	otpdomain "github.com/dont-wait/anomaly/internal/domain/otp"
)

const (
	maxVerifyAttempts = 5
	attemptWindow     = 5 * time.Minute
)

type VerifyOTPCommand struct {
	Email string
	Code  string
}

type VerifyOTPHandler struct {
	store OTPStore
	audit audit.Recorder
}

func NewVerifyOTPHandler(store OTPStore, recorders ...audit.Recorder) *VerifyOTPHandler {
	var recorder audit.Recorder
	if len(recorders) > 0 {
		recorder = recorders[0]
	}
	return &VerifyOTPHandler{store: store, audit: recorder}
}

// Handle kiểm tra mã OTP. compare-and-delete: đúng → consume,
// sai → giữ key, key miss → ErrOTPExpired. Không lookup account,
// không flip IsVerify. Sau 5 lần sai liên tiếp thì invalidate OTP.
func (h *VerifyOTPHandler) Handle(ctx context.Context, cmd VerifyOTPCommand) error {
	email, err := otpdomain.ValidateEmail(cmd.Email)
	if err != nil {
		return err
	}
	if err := otpdomain.ValidateCode(cmd.Code); err != nil {
		return err
	}

	// Consume trước: key miss trả ErrOTPExpired ngay và KHÔNG đếm attempt,
	// nếu không attacker có thể bơm counter khống trước khi nạn nhân
	// request OTP và khoá luôn mã thật.
	consumed, err := h.store.Consume(ctx, email, cmd.Code)
	if err != nil {
		return err
	}
	if consumed {
		_ = h.store.DelKey(ctx, attemptsKey(email))
		return nil
	}

	// Chỉ đếm khi OTP còn sống mà code sai.
	attempts, err := h.store.IncrAttempts(ctx, attemptsKey(email), attemptWindow)
	if err != nil {
		return err
	}
	if attempts > maxVerifyAttempts {
		_ = h.store.Del(ctx, email)
		return otpdomain.ErrOTPExpired
	}
	return otpdomain.ErrOTPInvalid
}

// HandleWithAudit is the email OTP path used by the HTTP handler. The legacy
// Handle method remains available for callers that only need the OTP result.
// Audit failures are returned as infrastructure errors rather than being
// misreported as an invalid or expired OTP.
func (h *VerifyOTPHandler) HandleWithAudit(ctx context.Context, cmd VerifyOTPCommand) error {
	// Keep validation semantics identical to Handle, but record only outcomes
	// that reached the Redis challenge state machine.
	email, err := otpdomain.ValidateEmail(cmd.Email)
	if err != nil {
		return err
	}
	if err := otpdomain.ValidateCode(cmd.Code); err != nil {
		return err
	}

	operationID := audit.NewEvent(audit.EmailOTPVerificationSucceeded, "", time.Now()).OperationID
	consumed, err := h.store.Consume(ctx, email, cmd.Code)
	if err != nil {
		if errors.Is(err, otpdomain.ErrOTPExpired) {
			if auditErr := h.recordEmailAudit(ctx, audit.EmailOTPExpired, operationID, email, nil, "expired"); auditErr != nil {
				return auditErr
			}
		}
		return err
	}
	if consumed {
		_ = h.store.DelKey(ctx, attemptsKey(email))
		if err := h.recordEmailAudit(ctx, audit.EmailOTPVerificationSucceeded, operationID, email, nil, ""); err != nil {
			return err
		}
		return nil
	}

	attempts, err := h.store.IncrAttempts(ctx, attemptsKey(email), attemptWindow)
	if err != nil {
		return err
	}
	if attempts > maxVerifyAttempts {
		_ = h.store.Del(ctx, email)
		if err := h.recordEmailAudit(ctx, audit.EmailOTPAttemptsExceeded, operationID, email, intPointer(0), "attempts_exceeded"); err != nil {
			return err
		}
		return otpdomain.ErrOTPExpired
	}
	left := int(maxVerifyAttempts - attempts)
	if err := h.recordEmailAudit(ctx, audit.EmailOTPVerificationFailed, operationID, email, &left, "invalid_code"); err != nil {
		return err
	}
	return otpdomain.ErrOTPInvalid
}

func (h *VerifyOTPHandler) recordEmailAudit(ctx context.Context, eventType, operationID, email string, attemptsLeft *int, reason string) error {
	if h.audit == nil {
		return nil
	}
	event := audit.NewEvent(eventType, operationID, time.Now().UTC())
	event.EmailHash = audit.HashEmail(email)
	event.AttemptsLeft = attemptsLeft
	event.ReasonCode = reason
	return h.audit.Record(ctx, event)
}

func intPointer(value int) *int { return &value }
