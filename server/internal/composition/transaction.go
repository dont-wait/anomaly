package composition

import (
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	apptransaction "github.com/dont-wait/anomaly/internal/application/transaction"
	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/domain/audit"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	"github.com/dont-wait/anomaly/internal/infrastructure/mail"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	otpinfra "github.com/dont-wait/anomaly/internal/infrastructure/otp"
	handlertransaction "github.com/dont-wait/anomaly/internal/presentation/http/handler/transaction"
)

func newTransferMailSender(smtpCfg *domain.SMTPConfig, logger zerolog.Logger) otpinfra.TransferMailSender {
	var sender otpinfra.TransferMailSender
	sender, err := mail.NewSMTPSender(smtpCfg)
	if err != nil {
		logger.Warn().Err(err).Msg("smtp not configured, transaction otp will fail closed")
		sender = mail.NewDisabledSender(err)
	}
	return sender
}

func NewTransactionHandler(repo *eventstore.BankRepository, contacts *mongorepo.RecipientContactRepository, rdb *redis.Client, smtpCfg *domain.SMTPConfig, logger zerolog.Logger, receiptOutbox ...handlertransaction.ReceiptOutbox) *handlertransaction.Handler {
	sender := newTransferMailSender(smtpCfg, logger)
	return handlertransaction.NewHandler(logger, repo, otpinfra.NewTransferOTPStore(rdb, sender), sender, contacts, receiptOutbox...)
}

func NewTransactionHandlerWithAudit(repo *eventstore.BankRepository, contacts *mongorepo.RecipientContactRepository, rdb *redis.Client, smtpCfg *domain.SMTPConfig, logger zerolog.Logger, recorder audit.Recorder, receiptOutbox ...handlertransaction.ReceiptOutbox) *handlertransaction.Handler {
	handler := NewTransactionHandler(repo, contacts, rdb, smtpCfg, logger, receiptOutbox...)
	return handler.WithAudit(recorder)
}

func NewReceiptWorker(outbox apptransaction.ReceiptOutbox, smtpCfg *domain.SMTPConfig, logger zerolog.Logger) *apptransaction.ReceiptWorker {
	return apptransaction.NewReceiptWorker(outbox, newTransferMailSender(smtpCfg, logger), logger)
}
