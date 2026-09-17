package otp

import (
	"context"
	"time"

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
}

func NewVerifyOTPHandler(store OTPStore) *VerifyOTPHandler {
	return &VerifyOTPHandler{store: store}
}

// Handle kiểm tra mã OTP.原子 compare-and-delete: đúng → consume,
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
