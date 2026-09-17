package otp

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	maildomain "github.com/dont-wait/anomaly/internal/domain/mail"
	otpdomain "github.com/dont-wait/anomaly/internal/domain/otp"
)

const requestCooldown = 30 * time.Second

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
	if err := h.mail.Send(ctx, maildomain.MailMessage{To: email, Subject: subject, Text: text, HTML: html}); err != nil {
		h.log.Error().Err(err).Str("email", email).Msg("send otp email failed")
		// Gỡ cooldown để retry trong 30s còn gửi lại được thay vì bị
		// throttle im lặng.
		_ = h.store.DelKey(ctx, cooldownKey(email))
		return err
	}

	if err := h.store.Set(ctx, email, code, otpdomain.TTL); err != nil {
		h.log.Error().Err(err).Str("email", email).Msg("store otp after send failed: code delivered but not persisted")
		_ = h.store.DelKey(ctx, cooldownKey(email))
		return err
	}

	// OTP mới phát hành -> reset bộ đếm nhập sai của lần trước.
	_ = h.store.DelKey(ctx, attemptsKey(email))

	h.log.Info().Str("email", email).Msg("otp sent")
	return nil
}
