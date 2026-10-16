package transaction

import (
	"context"
	"errors"
	"testing"
	"time"

	maildomain "github.com/dont-wait/anomaly/internal/domain/mail"
	"github.com/rs/zerolog"
)

type receiptOutboxFake struct {
	item       maildomain.ReceiptOutboxItem
	claimed    bool
	markedSent bool
	markedFail bool
	nextRetry  time.Time
	markError  error
}

func (f *receiptOutboxFake) Enqueue(context.Context, string, maildomain.MailMessage) error {
	return nil
}

func (f *receiptOutboxFake) Claim(context.Context, time.Time) (maildomain.ReceiptOutboxItem, error) {
	if f.claimed {
		return maildomain.ReceiptOutboxItem{}, nil
	}
	f.claimed = true
	return f.item, nil
}

func (f *receiptOutboxFake) MarkSent(context.Context, string, string, time.Time) error {
	f.markedSent = true
	return f.markError
}

func (f *receiptOutboxFake) MarkFailed(_ context.Context, _ string, _ string, _ error, next time.Time) error {
	f.markedFail = true
	f.nextRetry = next
	return f.markError
}

type receiptSenderFake struct {
	err error
}

func (f receiptSenderFake) Send(context.Context, maildomain.MailMessage) error {
	return f.err
}

func TestReceiptWorkerMarksSuccessfulDelivery(t *testing.T) {
	outbox := &receiptOutboxFake{item: maildomain.ReceiptOutboxItem{ID: "transfer-1", LockID: "lock-1"}}
	worker := NewReceiptWorker(outbox, receiptSenderFake{}, zerolog.Nop())

	worker.ProcessOnce(context.Background())

	if !outbox.markedSent {
		t.Fatal("expected successful receipt to be marked sent")
	}
	if outbox.markedFail {
		t.Fatal("did not expect failed state for successful delivery")
	}
}

func TestReceiptWorkerSchedulesRetryAfterDeliveryFailure(t *testing.T) {
	outbox := &receiptOutboxFake{item: maildomain.ReceiptOutboxItem{ID: "transfer-1", LockID: "lock-1", Attempts: 2}}
	worker := NewReceiptWorker(outbox, receiptSenderFake{err: errors.New("smtp unavailable")}, zerolog.Nop())

	before := time.Now().Add(time.Minute)
	worker.ProcessOnce(context.Background())

	if !outbox.markedFail {
		t.Fatal("expected failed receipt to be scheduled for retry")
	}
	if outbox.nextRetry.Before(before) {
		t.Fatalf("retry scheduled too early: %v", outbox.nextRetry)
	}
}

func TestReceiptRetryDelayCapsAtOneHour(t *testing.T) {
	if got := receiptRetryDelay(100); got != time.Hour {
		t.Fatalf("receiptRetryDelay(100) = %s, want 1h", got)
	}
}
