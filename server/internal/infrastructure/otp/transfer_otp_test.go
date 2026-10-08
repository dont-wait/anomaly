package otp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	maildomain "github.com/dont-wait/anomaly/internal/domain/mail"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestTransferOTPVerifyTransitionsToRetryableAuthorization(t *testing.T) {
	store, rdb := newTransferOTPTestStore(t)
	ctx := context.Background()
	id := "retryable"
	setTransferOTPChallenge(t, rdb, id, "123456", 30*time.Second)

	left, err := store.Verify(ctx, id, "123456")
	if err != nil || left != transferOTPAttempts {
		t.Fatalf("first Verify() = (%d, %v), want (%d, nil)", left, err, transferOTPAttempts)
	}
	if rdb.Exists(ctx, transferOTPKey(id), transferOTPAttemptsKey(id)).Val() != 0 {
		t.Fatal("successful verification did not delete challenge state")
	}
	verifiedTTL := rdb.PTTL(ctx, transferOTPVerifiedKey(id)).Val()
	if verifiedTTL <= 0 || verifiedTTL > 30*time.Second {
		t.Fatalf("verified TTL = %v, want remaining challenge TTL", verifiedTTL)
	}

	left, err = store.Verify(ctx, id, "not-even-a-code")
	if err != nil || left != transferOTPAttempts {
		t.Fatalf("retry Verify() = (%d, %v), want (%d, nil)", left, err, transferOTPAttempts)
	}
}

func TestTransferOTPVerifyFailuresAreBoundedAndExpireAtMaximum(t *testing.T) {
	store, rdb := newTransferOTPTestStore(t)
	ctx := context.Background()
	id := "failures"
	setTransferOTPChallenge(t, rdb, id, "123456", 30*time.Second)

	for attempt, wantLeft := range []int{2, 1} {
		left, err := store.Verify(ctx, id, "654321")
		if !errors.Is(err, ErrTransferOTPInvalid) || left != wantLeft {
			t.Fatalf("Verify() attempt %d = (%d, %v), want (%d, invalid)", attempt+1, left, err, wantLeft)
		}
		challengeTTL := rdb.PTTL(ctx, transferOTPKey(id)).Val()
		attemptTTL := rdb.PTTL(ctx, transferOTPAttemptsKey(id)).Val()
		if attemptTTL <= 0 || attemptTTL > challengeTTL {
			t.Fatalf("attempt TTL = %v, challenge TTL = %v", attemptTTL, challengeTTL)
		}
	}

	left, err := store.Verify(ctx, id, "654321")
	if !errors.Is(err, ErrTransferOTPExceeded) || left != 0 {
		t.Fatalf("Verify() at maximum = (%d, %v), want (0, exceeded)", left, err)
	}
	if rdb.Exists(ctx, transferOTPKey(id), transferOTPAttemptsKey(id), transferOTPVerifiedKey(id)).Val() != 0 {
		t.Fatal("maximum attempts did not clear transfer OTP state")
	}
	if _, err := store.Verify(ctx, id, "123456"); !errors.Is(err, ErrTransferOTPExpired) {
		t.Fatalf("Verify() after maximum error = %v, want expired", err)
	}
}

func TestTransferOTPConcurrentFailuresEnforceAttemptLimit(t *testing.T) {
	store, rdb := newTransferOTPTestStore(t)
	ctx := context.Background()
	id := "concurrent-failures"
	setTransferOTPChallenge(t, rdb, id, "123456", 30*time.Second)

	type result struct {
		left int
		err  error
	}
	results := make(chan result, transferOTPAttempts)
	var wg sync.WaitGroup
	for range transferOTPAttempts {
		wg.Go(func() {
			left, err := store.Verify(ctx, id, "654321")
			results <- result{left: left, err: err}
		})
	}
	wg.Wait()
	close(results)

	invalidLeft := map[int]bool{}
	exceeded := 0
	for result := range results {
		switch {
		case errors.Is(result.err, ErrTransferOTPInvalid):
			invalidLeft[result.left] = true
		case errors.Is(result.err, ErrTransferOTPExceeded):
			exceeded++
		default:
			t.Fatalf("concurrent Verify() = (%d, %v)", result.left, result.err)
		}
	}
	if exceeded != 1 || !invalidLeft[1] || !invalidLeft[2] || len(invalidLeft) != 2 {
		t.Fatalf("exceeded/invalid attempts = %d/%v, want one exceeded and attempts left 2,1", exceeded, invalidLeft)
	}
	if rdb.Exists(ctx, transferOTPKey(id), transferOTPAttemptsKey(id), transferOTPVerifiedKey(id)).Val() != 0 {
		t.Fatal("concurrent attempt exhaustion did not clear transfer OTP state")
	}
}

func TestTransferOTPIssueAndConsumeResetAllState(t *testing.T) {
	store, rdb := newTransferOTPTestStore(t)
	store.mail = transferOTPTestMailSender{}
	ctx := context.Background()
	id := "cleanup"
	for _, key := range []string{transferOTPKey(id), transferOTPAttemptsKey(id), transferOTPVerifiedKey(id)} {
		if err := rdb.Set(ctx, key, "stale", time.Minute).Err(); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := store.Issue(ctx, id, "person@example.com"); err != nil {
		t.Fatal(err)
	}
	if got := rdb.Get(ctx, transferOTPKey(id)).Val(); got == "" || got == "stale" {
		t.Fatalf("challenge = %q, want newly issued code", got)
	}
	if rdb.Exists(ctx, transferOTPAttemptsKey(id), transferOTPVerifiedKey(id)).Val() != 0 {
		t.Fatal("issue did not clear stale attempts and verified state")
	}

	if err := store.Consume(ctx, id); err != nil {
		t.Fatal(err)
	}
	if rdb.Exists(ctx, transferOTPKey(id), transferOTPAttemptsKey(id), transferOTPVerifiedKey(id)).Val() != 0 {
		t.Fatal("Consume() did not clear all transfer OTP state")
	}
}

type transferOTPTestMailSender struct{}

func (transferOTPTestMailSender) Send(context.Context, maildomain.MailMessage) error { return nil }

func TestTransferOTPVerifyMissingChallengeIsExpired(t *testing.T) {
	store, _ := newTransferOTPTestStore(t)
	left, err := store.Verify(context.Background(), "missing", "123456")
	if !errors.Is(err, ErrTransferOTPExpired) || left != 0 {
		t.Fatalf("Verify() = (%d, %v), want (0, expired)", left, err)
	}
}

func newTransferOTPTestStore(t *testing.T) (*TransferOTPStore, *redis.Client) {
	t.Helper()
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:7-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForLog("Ready to accept connections").WithStartupTimeout(time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := container.MappedPort(ctx, "6379/tcp")
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: fmt.Sprintf("%s:%s", host, port.Port())})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewTransferOTPStore(rdb, nil), rdb
}

func setTransferOTPChallenge(t *testing.T, rdb *redis.Client, id, code string, ttl time.Duration) {
	t.Helper()
	if err := rdb.Set(context.Background(), transferOTPKey(id), code, ttl).Err(); err != nil {
		t.Fatal(err)
	}
}
