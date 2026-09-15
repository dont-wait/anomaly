package otp

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	maildomain "github.com/dont-wait/anomaly/internal/domain/mail"
	otpdomain "github.com/dont-wait/anomaly/internal/domain/otp"
)

const requestCooldown = 30 * time.Second

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

	cooldownKey := "otp:cooldown:" + email
	allowed, err := h.store.SetCooldown(ctx, cooldownKey, requestCooldown)
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
		return err
	}

	if err := h.store.Set(ctx, email, code, otpdomain.TTL); err != nil {
		h.log.Error().Err(err).Str("email", email).Msg("store otp after send failed")
		return err
	}

	h.log.Info().Str("email", email).Msg("otp sent")
	return nil
}
