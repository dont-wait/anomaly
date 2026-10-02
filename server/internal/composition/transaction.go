package composition

import (
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/infrastructure/mail"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	otpinfra "github.com/dont-wait/anomaly/internal/infrastructure/otp"
	handlertransaction "github.com/dont-wait/anomaly/internal/presentation/http/handler/transaction"
)

func NewTransactionHandler(client *mongodrv.Client, dbName string, rdb *redis.Client, smtpCfg *domain.SMTPConfig, logger zerolog.Logger) *handlertransaction.Handler {
	var sender otpinfra.TransferMailSender
	sender, err := mail.NewSMTPSender(smtpCfg)
	if err != nil {
		logger.Warn().Err(err).Msg("smtp not configured, transaction otp will fail closed")
		sender = mail.NewDisabledSender(err)
	}
	return handlertransaction.NewHandler(logger, mongorepo.NewTransferRepository(client, dbName), otpinfra.NewTransferOTPStore(rdb, sender))
}
