package kafka

import (
	"context"
	"errors"
	"testing"

	kafkago "github.com/segmentio/kafka-go"
)

func TestConsumerDoesNotCommitWhenHandlerFails(t *testing.T) {
	reader := &consumerReaderOnceFake{message: kafkago.Message{Offset: 7}}
	consumer := &Consumer{reader: reader}
	storageErr := errors.New("mongo unavailable")

	err := consumer.Consume(context.Background(), func(context.Context, kafkago.Message) error {
		return storageErr
	})
	if !errors.Is(err, storageErr) {
		t.Fatalf("handler error = %v, want %v", err, storageErr)
	}
	if reader.commit != 0 {
		t.Fatalf("commits = %d, want 0", reader.commit)
	}
}

func TestConsumerCommitsOnlyAfterHandlerSucceeds(t *testing.T) {
	reader := &consumerReaderOnceFake{message: kafkago.Message{Offset: 8}}
	consumer := &Consumer{reader: reader}

	if err := consumer.Consume(context.Background(), func(context.Context, kafkago.Message) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("consume error = %v, want context cancellation after one message", err)
	}
	if reader.commit != 1 {
		t.Fatalf("commits = %d, want 1", reader.commit)
	}
}

type consumerReaderOnceFake struct {
	message kafkago.Message
	commit  int
	read    bool
}

func (f *consumerReaderOnceFake) FetchMessage(ctx context.Context) (kafkago.Message, error) {
	if f.read {
		return kafkago.Message{}, context.Canceled
	}
	f.read = true
	return f.message, nil
}

func (f *consumerReaderOnceFake) CommitMessages(context.Context, ...kafkago.Message) error {
	f.commit++
	return nil
}

func (*consumerReaderOnceFake) Close() error { return nil }
