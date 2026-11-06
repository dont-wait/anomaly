package otp

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	maildomain "github.com/dont-wait/anomaly/internal/domain/mail"
	otpdomain "github.com/dont-wait/anomaly/internal/domain/otp"
)

const requestCooldown = 30 * time.Second

// persistTimeout bounds Redis writes that must finish even after the client
// disconnects, since the email may already be on its way.
const persistTimeout = 5 * time.Second

func cooldownKey(email string) string { return "otp:cooldown:" + email }

func attemptsKey(email string) string { return "otp:attempts:" + email }

type RequestOTPCommand struct {
	Email string
}

type RequestOTPCommandHandler struct {
	store OTPStore
	mail  MailSender
	log   zerolog.Logger
}

func NewRequestOTPCommandHandler(store OTPStore, mail MailSender, log zerolog.Logger) *RequestOTPCommandHandler {
	return &RequestOTPCommandHandler{store: store, mail: mail, log: log}
}

func (h *RequestOTPCommandHandler) Handle(ctx context.Context, cmd RequestOTPCommand) error {
	email, err := otpdomain.ValidateEmail(cmd.Email)
	if err != nil {
		return err
	}

	allowed, err := h.store.SetCooldown(ctx, cooldownKey(email), requestCooldown)
	if err != nil {
		return err
	}
	if !allowed {
		h.log.Warn().Str("email", email).Msg("otp request throttled")
		return nil
	}

	code, err := otpdomain.GenerateCode()
	if err != nil {
		return err
	}

	subject, text, html, err := renderOTPEmail(code)
	if err != nil {
		return err
	}
	sendErr := h.mail.Send(ctx, maildomain.MailMessage{To: email, Subject: subject, Text: text, HTML: html})

	// Từ đây không dùng ctx của request nữa: client có thể đã ngắt kết nối
	// trong lúc SMTP chậm, nhưng mail vẫn có thể đã tới hộp thư. Nếu dùng ctx
	// đã cancel, mã đã gửi sẽ không được lưu và người dùng nhận mã vô dụng.
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), persistTimeout)
	defer cancel()

	if sendErr != nil {
		h.log.Error().Err(sendErr).Str("email", email).Msg("send otp email failed")
		// Gỡ cooldown để retry trong 30s còn gửi lại được thay vì bị
		// throttle im lặng.
		_ = h.store.DelKey(persistCtx, cooldownKey(email))
		return sendErr
	}

	if err := h.store.Set(persistCtx, email, code, otpdomain.TTL); err != nil {
		h.log.Error().Err(err).Str("email", email).Msg("store otp after send failed: code delivered but not persisted")
		_ = h.store.DelKey(persistCtx, cooldownKey(email))
		return err
	}

	// OTP mới phát hành -> reset bộ đếm nhập sai của lần trước.
	_ = h.store.DelKey(persistCtx, attemptsKey(email))

	h.log.Info().Str("email", email).Msg("otp sent")
	return nil
}
