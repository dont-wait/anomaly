package composition

import (
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	appotp "github.com/dont-wait/anomaly/internal/application/otp"
	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/infrastructure/mail"
	otpinfra "github.com/dont-wait/anomaly/internal/infrastructure/otp"
	handlerotp "github.com/dont-wait/anomaly/internal/presentation/http/handler/otp"
)

// NewOTPHandler wiring OTP: Redis store + SMTP sender + usecases + handler.
// SMTP chưa cấu hình thì fail-closed theo từng request (sender luôn lỗi,
// request trả 500) thay vì crash cả API lúc startup.
func NewOTPHandler(rdb *redis.Client, smtpCfg *domain.SMTPConfig, logger zerolog.Logger) *handlerotp.Handler {
	store := otpinfra.NewRedisStore(rdb)

	var sender appotp.MailSender
	sender, err := mail.NewSMTPSender(smtpCfg)
	if err != nil {
		logger.Warn().Err(err).Msg("smtp not configured, otp requests will fail closed")
		sender = mail.NewDisabledSender(err)
	}

	return handlerotp.NewHandler(
		logger,
		appotp.NewRequestOTPCommandHandler(store, sender, logger),
		appotp.NewVerifyOTPHandler(store),
	)
}
