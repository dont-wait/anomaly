package seeder

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestRegistryRunsAllSeedersInNameOrder(t *testing.T) {
	r := registry{}
	var calls []string
	for _, name := range []string{"020_second", "010_first", "030_third"} {
		r.register(name, func(context.Context, Dependencies) error {
			calls = append(calls, name)
			return nil
		})
	}
	if err := r.run(context.Background(), Dependencies{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"010_first", "020_second", "030_third"}) {
		t.Fatalf("unexpected order: %v", calls)
	}
}

func TestRegistryStopsAndIdentifiesFailedSeeder(t *testing.T) {
	r := registry{}
	failure := errors.New("database failure")
	r.register("010_failed", func(context.Context, Dependencies) error { return failure })
	r.register("020_skipped", func(context.Context, Dependencies) error { t.Fatal("ran after failure"); return nil })
	err := r.run(context.Background(), Dependencies{})
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "010_failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRegistryRejectsDuplicateNames(t *testing.T) {
	r := registry{}
	run := func(context.Context, Dependencies) error { return nil }
	r.register("accounts", run)
	defer func() {
		if recover() == nil {
			t.Fatal("expected duplicate registration to fail")
		}
	}()
	r.register("accounts", run)
}

func TestRegistryStopsOnCancellation(t *testing.T) {
	r := registry{}
	r.register("accounts", func(context.Context, Dependencies) error { t.Fatal("ran despite cancellation"); return nil })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.run(ctx, Dependencies{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected error: %v", err)
	}
}
