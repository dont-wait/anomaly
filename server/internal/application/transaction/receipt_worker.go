package transaction

import (
	"context"
	"time"

	maildomain "github.com/dont-wait/anomaly/internal/domain/mail"
	"github.com/rs/zerolog"
)

const (
	receiptWorkerInterval = 15 * time.Second
	receiptSendTimeout    = 30 * time.Second
	receiptMaxBackoff     = time.Hour
)

// ReceiptOutbox stores receipts durably and claims each receipt for one sender
// at a time. The lock id prevents a stale worker from completing a newer claim.
type ReceiptOutbox interface {
	Enqueue(context.Context, string, maildomain.MailMessage) error
	Claim(context.Context, time.Time) (maildomain.ReceiptOutboxItem, error)
	MarkSent(context.Context, string, string, time.Time) error
	MarkFailed(context.Context, string, string, error, time.Time) error
}

type ReceiptMailSender interface {
	Send(context.Context, maildomain.MailMessage) error
}

type ReceiptWorker struct {
	outbox ReceiptOutbox
	sender ReceiptMailSender
	logger zerolog.Logger
}

func NewReceiptWorker(outbox ReceiptOutbox, sender ReceiptMailSender, logger zerolog.Logger) *ReceiptWorker {
	return &ReceiptWorker{outbox: outbox, sender: sender, logger: logger}
}

// Run continuously drains the receipt outbox. Delivery failures stay in the
// outbox and are retried with exponential backoff, so a temporary SMTP outage
// cannot lose a receipt or block the transfer confirmation request.
func (w *ReceiptWorker) Run(ctx context.Context) {
	w.ProcessOnce(ctx)
	ticker := time.NewTicker(receiptWorkerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.ProcessOnce(ctx)
		}
	}
}

// ProcessOnce drains currently claimable receipts. It is exported for
// operators and tests that want to trigger a delivery pass immediately.
func (w *ReceiptWorker) ProcessOnce(ctx context.Context) {
	if w.outbox == nil || w.sender == nil {
		return
	}
	for range 100 {
		item, err := w.outbox.Claim(ctx, time.Now().UTC())
		if err != nil {
			w.logger.Error().Err(err).Msg("claim transfer receipt failed")
			return
		}
		if item.ID == "" {
			return
		}

		sendCtx, cancel := context.WithTimeout(ctx, receiptSendTimeout)
		err = w.sender.Send(sendCtx, maildomain.MailMessage{
			To:      item.To,
			Subject: item.Subject,
			Text:    item.Text,
			HTML:    item.HTML,
		})
		cancel()
		if err != nil {
			nextAttemptAt := time.Now().UTC().Add(receiptRetryDelay(item.Attempts))
			if markErr := w.outbox.MarkFailed(ctx, item.ID, item.LockID, err, nextAttemptAt); markErr != nil {
				w.logger.Error().Err(markErr).Str("transferId", item.ID).
					Str("deliveryError", err.Error()).Msg("receipt retry state update failed")
			}
			continue
		}
		if err := w.outbox.MarkSent(ctx, item.ID, item.LockID, time.Now().UTC()); err != nil {
			w.logger.Error().Err(err).Str("transferId", item.ID).Msg("mark transfer receipt sent failed")
		}
	}
}

func receiptRetryDelay(attempts int) time.Duration {
	if attempts <= 0 {
		return time.Minute
	}
	delay := time.Minute
	for attempt := 1; attempt < attempts; attempt++ {
		if delay >= receiptMaxBackoff/2 {
			return receiptMaxBackoff
		}
		delay *= 2
	}
	return delay
}
