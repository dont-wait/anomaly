package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLoadCheckpointRetriesPositionFailure(t *testing.T) {
	wantErr := errors.New("checkpoint unavailable")
	attempts := 0
	logged := 0

	position, found, err := loadCheckpoint(context.Background(), time.Millisecond, func(context.Context) (uint64, bool, error) {
		attempts++
		if attempts == 1 {
			return 0, false, wantErr
		}
		return 42, true, nil
	}, func(err error) {
		logged++
		if !errors.Is(err, wantErr) {
			t.Fatalf("logged error = %v, want %v", err, wantErr)
		}
	})
	if err != nil {
		t.Fatalf("loadCheckpoint() error = %v", err)
	}
	if position != 42 || !found {
		t.Fatalf("loadCheckpoint() = (%d, %t), want (42, true)", position, found)
	}
	if attempts != 2 || logged != 1 {
		t.Fatalf("attempts/logged = %d/%d, want 2/1", attempts, logged)
	}
}

func TestLoadCheckpointBackoffStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempted := make(chan struct{})
	result := make(chan error, 1)

	go func() {
		_, _, err := loadCheckpoint(ctx, time.Hour, func(context.Context) (uint64, bool, error) {
			close(attempted)
			return 0, false, errors.New("checkpoint unavailable")
		}, func(error) {})
		result <- err
	}()

	<-attempted
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("loadCheckpoint() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("loadCheckpoint did not stop during retry backoff")
	}
}
