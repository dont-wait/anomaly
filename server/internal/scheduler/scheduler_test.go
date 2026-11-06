package scheduler

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func testLogger() *zerolog.Logger {
	log := zerolog.Nop()
	return &log
}

func TestNewValidatesJobs(t *testing.T) {
	good := Job{Name: "loan", Interval: time.Hour, Timeout: time.Minute, Run: func(context.Context, time.Time) error { return nil }}
	tests := []struct {
		name string
		jobs []Job
	}{
		{name: "no jobs"},
		{name: "empty name", jobs: []Job{{Interval: time.Hour, Timeout: time.Minute, Run: good.Run}}},
		{name: "duplicate names", jobs: []Job{good, good}},
		{name: "zero interval", jobs: []Job{{Name: "a", Timeout: time.Minute, Run: good.Run}}},
		{name: "zero timeout", jobs: []Job{{Name: "a", Interval: time.Hour, Run: good.Run}}},
		{name: "nil callback", jobs: []Job{{Name: "a", Interval: time.Hour, Timeout: time.Minute}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New(testLogger(), tt.jobs); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	if _, err := New(nil, []Job{good}); err == nil {
		t.Fatal("expected missing logger error")
	}
	if _, err := New(testLogger(), []Job{good}, WithNow(nil)); err == nil {
		t.Fatal("expected nil clock error")
	}
}

func TestRunOnceJoinsFailuresAndBoundsEachJob(t *testing.T) {
	firstError := errors.New("first error")
	var ranAfterFailure atomic.Bool
	fixedNow := time.Date(2026, 11, 5, 2, 0, 0, 0, time.UTC)
	s, err := New(testLogger(), []Job{
		{Name: "first", Interval: time.Hour, Timeout: time.Second, Run: func(_ context.Context, now time.Time) error {
			if !now.Equal(fixedNow) {
				t.Fatalf("unexpected time %s", now)
			}
			return firstError
		}},
		{Name: "second", Interval: time.Hour, Timeout: 10 * time.Millisecond, Run: func(ctx context.Context, _ time.Time) error {
			<-ctx.Done()
			return ctx.Err()
		}},
		{Name: "third", Interval: time.Hour, Timeout: time.Second, Run: func(context.Context, time.Time) error {
			ranAfterFailure.Store(true)
			return nil
		}},
	}, WithNow(func() time.Time { return fixedNow }))
	if err != nil {
		t.Fatal(err)
	}
	joined := s.RunOnce(context.Background())
	if !errors.Is(joined, firstError) || !errors.Is(joined, context.DeadlineExceeded) || !ranAfterFailure.Load() {
		t.Fatalf("missing failures or skipped remaining job: %v", joined)
	}
	if !strings.Contains(joined.Error(), "first") || !strings.Contains(joined.Error(), "second") {
		t.Fatalf("missing job names: %v", joined)
	}
}

func TestStartRunsImmediatelyTicksAndStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	s, err := New(testLogger(), []Job{{
		Name: "quick", Interval: 10 * time.Millisecond, Timeout: time.Second,
		Run: func(context.Context, time.Time) error {
			if calls.Add(1) >= 2 {
				cancel()
			}
			return nil
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		s.Start(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not tick and shut down")
	}
	if calls.Load() < 2 {
		t.Fatalf("expected immediate run and next tick, got %d", calls.Load())
	}
}
