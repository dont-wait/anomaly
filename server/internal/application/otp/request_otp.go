package otp

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"

	maildomain "github.com/dont-wait/anomaly/internal/domain/mail"
	otpdomain "github.com/dont-wait/anomaly/internal/domain/otp"
)

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

	code, err := otpdomain.GenerateCode()
	if err != nil {
		return err
	}

	if err := h.store.Set(ctx, email, code, otpdomain.TTL); err != nil {
		return err
	}

	subject := "Ma OTP cua ban / Your OTP code"
	body := fmt.Sprintf("Ma OTP cua ban la: %s. Ma het han sau 60 giay.\nYour OTP code is: %s. It expires in 60 seconds.\n", code, code)
	if err := h.mail.Send(ctx, maildomain.MailMessage{To: email, Subject: subject, Text: body}); err != nil {
		h.log.Error().Err(err).Str("email", email).Msg("send otp email failed, key removed")
		if delErr := h.store.Del(ctx, email); delErr != nil {
			h.log.Error().Err(delErr).Str("email", email).Msg("remove orphan otp key failed")
		}
		return err
	}

	h.log.Info().Str("email", email).Msg("otp sent")
	return nil
}
