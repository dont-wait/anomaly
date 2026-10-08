package composition

import (
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	"github.com/dont-wait/anomaly/internal/infrastructure/mail"
	otpinfra "github.com/dont-wait/anomaly/internal/infrastructure/otp"
	handlertransaction "github.com/dont-wait/anomaly/internal/presentation/http/handler/transaction"
)

func NewTransactionHandler(repo *eventstore.BankRepository, rdb *redis.Client, smtpCfg *domain.SMTPConfig, logger zerolog.Logger) *handlertransaction.Handler {
	var sender otpinfra.TransferMailSender
	sender, err := mail.NewSMTPSender(smtpCfg)
	if err != nil {
		logger.Warn().Err(err).Msg("smtp not configured, transaction otp will fail closed")
		sender = mail.NewDisabledSender(err)
	}
	return handlertransaction.NewHandler(logger, repo, otpinfra.NewTransferOTPStore(rdb, sender))
}
