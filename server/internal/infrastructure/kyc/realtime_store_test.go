package kyc

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func testRealtimeStore(t *testing.T) *RealtimeStore {
	t.Helper()
	binary := os.Getenv("KYC_TEST_REDIS_SERVER")
	if binary == "" {
		binary, _ = exec.LookPath("redis-server")
	}
	if binary == "" {
		t.Skip("redis-server required for integration test")
	}
	directory, err := os.MkdirTemp("/tmp", "kyc-redis-")
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(directory, "redis.sock")
	process := exec.Command(binary, "--port", "0", "--unixsocket", socket, "--save", "", "--appendonly", "no", "--loglevel", "warning")
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(&redis.Options{Network: "unix", Addr: socket})
	t.Cleanup(func() { client.Close(); process.Process.Kill(); process.Wait(); os.RemoveAll(directory) })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for client.Ping(ctx).Err() != nil {
		if ctx.Err() != nil {
			t.Fatal("Redis did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return &RealtimeStore{DB: client}
}

func TestRealtimePolicyIsAtomicAndSurvivesRenewedSession(t *testing.T) {
	s := testRealtimeStore(t)
	ctx := context.Background()
	attempt := func(key string) *Attempt {
		return &Attempt{AccountID: "account", Key: key, FrontHash: "front", BackHash: "back", Challenge: "TURN_HEAD_LEFT_RIGHT_BLINK", ExpiresAt: time.Now().Add(time.Minute)}
	}
	first, err := s.Create(ctx, attempt("key1"))
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := s.Create(ctx, attempt("key1"))
	if err != nil || duplicate.ID != first.ID {
		t.Fatalf("duplicate: %v", err)
	}
	changed := attempt("key1")
	changed.FrontHash = "different"
	if _, err := s.Create(ctx, changed); !errors.Is(err, ErrAttemptConflict) {
		t.Fatalf("changed image accepted: %v", err)
	}
	if _, err := s.Create(ctx, attempt("key2")); !errors.Is(err, ErrAttemptConflict) {
		t.Fatalf("parallel account attempt: %v", err)
	}
	var group sync.WaitGroup
	for range 10 {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := s.CompleteFailure(ctx, first, []byte(`{"decision":"RETRY_ALLOWED"}`), time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339)); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	count, ttl, err := s.Count(ctx, "account")
	if err != nil || count != 1 || ttl < 23*time.Hour {
		t.Fatalf("count=%d ttl=%v err=%v", count, ttl, err)
	}
	s.Release(ctx, first)
	for _, key := range []string{"key2", "key3"} {
		a, err := s.Create(ctx, attempt(key))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.CompleteFailure(ctx, a, []byte(`{"decision":"RETRY_ALLOWED"}`), time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
		s.Release(ctx, a)
	}
	// A new store instance represents a different API replica or renewed KYC token.
	replica := &RealtimeStore{DB: s.DB}
	if _, err := replica.Create(ctx, attempt("key4")); !errors.Is(err, ErrAttemptLimit) {
		t.Fatalf("renewed session reset policy: %v", err)
	}
}

func TestRealtimeTicketsLocksAndCancellation(t *testing.T) {
	s := testRealtimeStore(t)
	ctx := context.Background()
	a, err := s.Create(ctx, &Attempt{AccountID: "account", Key: "key", ExpiresAt: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Ticket(ctx, a, "one-use-secret"); err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumeTicket(ctx, a.ID, "one-use-secret"); err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumeTicket(ctx, a.ID, "one-use-secret"); err == nil {
		t.Fatal("ticket replay accepted")
	}
	token, err := s.Lock(ctx, a.ID)
	if err != nil || token == "" {
		t.Fatal("lock unavailable")
	}
	s.Unlock(ctx, a.ID, "wrong-owner")
	if next, _ := s.Lock(ctx, a.ID); next != "" {
		t.Fatal("foreign unlock released live lock")
	}
	s.Unlock(ctx, a.ID, token)
	if next, _ := s.Lock(ctx, a.ID); next == "" {
		t.Fatal("owner unlock failed")
	}
	s.DB.Set(ctx, "kyc:cancelled:"+a.ID, "1", time.Minute)
	if err := s.Save(ctx, a); !errors.Is(err, ErrAttemptConflict) {
		t.Fatal("cancelled attempt resurrected")
	}
}

func TestRealtimeGlobalLeaseBoundsTemporaryMedia(t *testing.T) {
	s := testRealtimeStore(t)
	p := DefaultPolicy()
	p.MaxConcurrent = 1
	s.Policy = p
	ctx := context.Background()
	first, err := s.Create(ctx, &Attempt{AccountID: "first", Key: "a", ExpiresAt: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	second := &Attempt{AccountID: "second", Key: "b", ExpiresAt: time.Now().Add(time.Minute)}
	if _, err := s.Create(ctx, second); !errors.Is(err, ErrAttemptCapacity) {
		t.Fatalf("global capacity exceeded: %v", err)
	}
	s.Release(ctx, first)
	if _, err := s.Create(ctx, second); err != nil {
		t.Fatalf("released slot unusable: %v", err)
	}
}

func TestMediaReconciliationQueueSurvivesReplicaAndAcknowledgesAfterProcessing(t *testing.T) {
	s := testRealtimeStore(t)
	ctx := context.Background()
	if err := s.QueueCleanup(ctx, "account", []string{"kyc/account/orphan"}); err != nil {
		t.Fatal(err)
	}
	records, err := s.PendingCleanup(ctx)
	if err != nil || len(records) != 0 {
		t.Fatal("cleanup ran before commit grace period")
	}
	ids, err := s.DB.ZRange(ctx, "kyc:cleanup:due", 0, -1).Result()
	if err != nil || len(ids) != 1 {
		t.Fatal("cleanup not durable")
	}
	s.DB.ZAdd(ctx, "kyc:cleanup:due", redis.Z{Score: float64(time.Now().Add(-time.Second).Unix()), Member: ids[0]})
	replica := &RealtimeStore{DB: s.DB}
	records, err = replica.PendingCleanup(ctx)
	if err != nil || len(records) != 1 || records[0].Keys[0] != "kyc/account/orphan" {
		t.Fatalf("pending cleanup = %#v/%v", records, err)
	}
	if err := replica.FinishCleanup(ctx, records[0].ID); err != nil {
		t.Fatal(err)
	}
	records, err = s.PendingCleanup(ctx)
	if err != nil || len(records) != 0 {
		t.Fatal("cleanup acknowledgement failed")
	}
}
