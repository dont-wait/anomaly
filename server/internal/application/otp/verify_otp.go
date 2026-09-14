package otp

import (
	"context"
	"crypto/subtle"

	otpdomain "github.com/dont-wait/anomaly/internal/domain/otp"
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

// Handle kiểm tra mã OTP. Đúng -> consume một lần (DEL key) và trả nil;
// sai -> giữ key cho TTL tự hết và trả ErrOTPInvalid;
// key miss -> ErrOTPExpired. Không lookup account, không flip IsVerify.
func (h *VerifyOTPHandler) Handle(ctx context.Context, cmd VerifyOTPCommand) error {
	email, err := otpdomain.ValidateEmail(cmd.Email)
	if err != nil {
		return err
	}
	if err := otpdomain.ValidateCode(cmd.Code); err != nil {
		return err
	}

	stored, err := h.store.Get(ctx, email)
	if err != nil {
		return err
	}

	if subtle.ConstantTimeCompare([]byte(stored), []byte(cmd.Code)) != 1 {
		return otpdomain.ErrOTPInvalid
	}

	if err := h.store.Del(ctx, email); err != nil {
		return err
	}
	return nil
}
