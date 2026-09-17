package otp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	maildomain "github.com/dont-wait/anomaly/internal/domain/mail"
	otpdomain "github.com/dont-wait/anomaly/internal/domain/otp"
)

type fakeStore struct {
	mu       sync.Mutex
	data     map[string]string
	cooldown map[string]time.Time
	attempts map[string]int64
	setN     int
	delN     int
	getE     error
	setE     error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		data:     map[string]string{},
		cooldown: map[string]time.Time{},
		attempts: map[string]int64{},
	}
}

func (s *fakeStore) Set(_ context.Context, email, code string, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.setE != nil {
		return s.setE
	}
	s.data[email] = code
	s.setN++
	return nil
}

func (s *fakeStore) Get(_ context.Context, email string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getE != nil {
		return "", s.getE
	}
	code, ok := s.data[email]
	if !ok {
		return "", otpdomain.ErrOTPExpired
	}
	return code, nil
}

func (s *fakeStore) Del(_ context.Context, email string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, email)
	s.delN++
	return nil
}

func (s *fakeStore) Consume(_ context.Context, email, code string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.data[email]
	if !ok {
		return false, otpdomain.ErrOTPExpired
	}
	if stored != code {
		return false, nil
	}
	delete(s.data, email)
	return true, nil
}

func (s *fakeStore) DelIfMatch(_ context.Context, email, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.data[email]
	if !ok || stored != code {
		return nil
	}
	delete(s.data, email)
	return nil
}

// SetCooldown mimics Redis SETNX với TTL: chỉ set được khi key chưa tồn
// tại hoặc đã hết hạn.
func (s *fakeStore) SetCooldown(_ context.Context, key string, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if exp, ok := s.cooldown[key]; ok && time.Now().Before(exp) {
		return false, nil
	}
	s.cooldown[key] = time.Now().Add(ttl)
	return true, nil
}

func (s *fakeStore) IncrAttempts(_ context.Context, key string, _ time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts[key]++
	return s.attempts[key], nil
}

func (s *fakeStore) DelKey(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cooldown, key)
	delete(s.attempts, key)
	return nil
}

func (s *fakeStore) attemptCount(key string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.attempts[key]
}

func (s *fakeStore) cooldownActive(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.cooldown[key]
	return ok && time.Now().Before(exp)
}

type fakeSender struct {
	mu      sync.Mutex
	sent    int
	sendErr error
	lastTo  string
	lastMsg maildomain.MailMessage
}

func (s *fakeSender) Send(_ context.Context, msg maildomain.MailMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sendErr != nil {
		return s.sendErr
	}
	s.sent++
	s.lastTo = msg.To
	s.lastMsg = msg
	return nil
}

func testLogger() zerolog.Logger {
	return zerolog.Nop()
}

func TestRequestOTPStoresAndSends(t *testing.T) {
	store := newFakeStore()
	sender := &fakeSender{}
	h := NewRequestOTPCommandHandler(store, sender, testLogger())

	if err := h.Handle(context.Background(), RequestOTPCommand{Email: "Alice@Example.COM"}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if sender.sent != 1 {
		t.Fatalf("sent = %d, want 1", sender.sent)
	}
	if sender.lastTo != "alice@example.com" {
		t.Fatalf("sent to = %q, want normalized email", sender.lastTo)
	}
	code, err := store.Get(context.Background(), "alice@example.com")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if err := otpdomain.ValidateCode(code); err != nil {
		t.Fatalf("stored code %q invalid: %v", code, err)
	}
}

func TestRequestOTPOverwritesPrevious(t *testing.T) {
	store := newFakeStore()
	sender := &fakeSender{}
	h := NewRequestOTPCommandHandler(store, sender, testLogger())
	ctx := context.Background()

	if err := h.Handle(ctx, RequestOTPCommand{Email: "a@b.co"}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	first, _ := store.Get(ctx, "a@b.co")
	// Prod chặn request thứ 2 trong 30s; bỏ cooldown để test đúng ý định
	// "phát hành lại thì ghi đè mã cũ".
	if err := store.DelKey(ctx, cooldownKey("a@b.co")); err != nil {
		t.Fatalf("DelKey() error = %v", err)
	}
	if err := h.Handle(ctx, RequestOTPCommand{Email: "a@b.co"}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if store.setN != 2 {
		t.Fatalf("setN = %d, want 2", store.setN)
	}
	second, _ := store.Get(ctx, "a@b.co")
	if second == "" || len(second) != otpdomain.OTPLength {
		t.Fatalf("overwritten code = %q, want 6 digits", second)
	}
	_ = first
}

func TestRequestOTPSendFailureDoesNotStoreKey(t *testing.T) {
	store := newFakeStore()
	sender := &fakeSender{sendErr: errors.New("smtp down")}
	h := NewRequestOTPCommandHandler(store, sender, testLogger())
	ctx := context.Background()

	if err := h.Handle(ctx, RequestOTPCommand{Email: "a@b.co"}); err == nil {
		t.Fatal("Handle() error = nil, want send error")
	}
	if store.setN != 0 {
		t.Fatalf("setN = %d, want 0 (no key stored on send failure)", store.setN)
	}
	if _, err := store.Get(ctx, "a@b.co"); err != otpdomain.ErrOTPExpired {
		t.Fatalf("Get() error = %v, want ErrOTPExpired (no key)", err)
	}
}

func TestRequestOTPSendFailureClearsCooldown(t *testing.T) {
	store := newFakeStore()
	sender := &fakeSender{sendErr: errors.New("smtp down")}
	h := NewRequestOTPCommandHandler(store, sender, testLogger())
	ctx := context.Background()

	if err := h.Handle(ctx, RequestOTPCommand{Email: "a@b.co"}); err == nil {
		t.Fatal("Handle() error = nil, want send error")
	}
	if store.cooldownActive(cooldownKey("a@b.co")) {
		t.Fatal("cooldown still active after send failure, retry would be throttled")
	}

	sender.sendErr = nil
	if err := h.Handle(ctx, RequestOTPCommand{Email: "a@b.co"}); err != nil {
		t.Fatalf("retry Handle() error = %v", err)
	}
	if sender.sent != 1 {
		t.Fatalf("sent = %d, want 1 on retry", sender.sent)
	}
}

func TestRequestOTPInvalidEmail(t *testing.T) {
	store := newFakeStore()
	sender := &fakeSender{}
	h := NewRequestOTPCommandHandler(store, sender, testLogger())

	if err := h.Handle(context.Background(), RequestOTPCommand{Email: "bad"}); err == nil {
		t.Fatal("Handle() error = nil, want invalid email")
	}
	if sender.sent != 0 {
		t.Fatalf("sent = %d, want 0", sender.sent)
	}
}

func TestVerifyOTPSuccessConsumesKey(t *testing.T) {
	store := newFakeStore()
	ctx := context.Background()
	if err := store.Set(ctx, "a@b.co", "123456", time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	h := NewVerifyOTPHandler(store)

	if err := h.Handle(ctx, VerifyOTPCommand{Email: "a@b.co", Code: "123456"}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if err := h.Handle(ctx, VerifyOTPCommand{Email: "a@b.co", Code: "123456"}); err != otpdomain.ErrOTPExpired {
		t.Fatalf("second Handle() error = %v, want ErrOTPExpired (consumed)", err)
	}
}

func TestVerifyOTPMismatchKeepsKey(t *testing.T) {
	store := newFakeStore()
	ctx := context.Background()
	if err := store.Set(ctx, "a@b.co", "123456", time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	h := NewVerifyOTPHandler(store)

	if err := h.Handle(ctx, VerifyOTPCommand{Email: "a@b.co", Code: "654321"}); err != otpdomain.ErrOTPInvalid {
		t.Fatalf("Handle() error = %v, want ErrOTPInvalid", err)
	}
	if _, err := store.Get(ctx, "a@b.co"); err != nil {
		t.Fatalf("Get() error = %v, want key kept after mismatch", err)
	}
}

func TestVerifyOTPMissingKeyExpired(t *testing.T) {
	h := NewVerifyOTPHandler(newFakeStore())

	if err := h.Handle(context.Background(), VerifyOTPCommand{Email: "a@b.co", Code: "123456"}); err != otpdomain.ErrOTPExpired {
		t.Fatalf("Handle() error = %v, want ErrOTPExpired", err)
	}
}

func TestVerifyOTPMissingKeyDoesNotCountAttempt(t *testing.T) {
	store := newFakeStore()
	h := NewVerifyOTPHandler(store)
	ctx := context.Background()

	for i := 0; i < maxVerifyAttempts+1; i++ {
		if err := h.Handle(ctx, VerifyOTPCommand{Email: "a@b.co", Code: "123456"}); err != otpdomain.ErrOTPExpired {
			t.Fatalf("Handle() error = %v, want ErrOTPExpired", err)
		}
	}
	if n := store.attemptCount(attemptsKey("a@b.co")); n != 0 {
		t.Fatalf("attempts = %d, want 0 (no counter before an OTP exists)", n)
	}

	// OTP phát hành sau đó vẫn dùng được, không bị khoá sẵn.
	if err := store.Set(ctx, "a@b.co", "123456", time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := h.Handle(ctx, VerifyOTPCommand{Email: "a@b.co", Code: "123456"}); err != nil {
		t.Fatalf("Handle() error = %v, want success", err)
	}
}

func TestVerifyOTPExhaustedAttemptsInvalidatesOTP(t *testing.T) {
	store := newFakeStore()
	h := NewVerifyOTPHandler(store)
	ctx := context.Background()
	if err := store.Set(ctx, "a@b.co", "123456", time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	for i := 0; i < maxVerifyAttempts; i++ {
		if err := h.Handle(ctx, VerifyOTPCommand{Email: "a@b.co", Code: "654321"}); err != otpdomain.ErrOTPInvalid {
			t.Fatalf("attempt %d error = %v, want ErrOTPInvalid", i+1, err)
		}
	}
	if err := h.Handle(ctx, VerifyOTPCommand{Email: "a@b.co", Code: "654321"}); err != otpdomain.ErrOTPExpired {
		t.Fatalf("Handle() error = %v, want ErrOTPExpired after exhausting attempts", err)
	}
	if _, err := store.Get(ctx, "a@b.co"); err != otpdomain.ErrOTPExpired {
		t.Fatalf("Get() error = %v, want OTP invalidated", err)
	}
}

func TestVerifyOTPBadInput(t *testing.T) {
	h := NewVerifyOTPHandler(newFakeStore())
	ctx := context.Background()

	if err := h.Handle(ctx, VerifyOTPCommand{Email: "bad", Code: "123456"}); err == nil {
		t.Fatal("Handle() error = nil, want invalid email")
	}
	if err := h.Handle(ctx, VerifyOTPCommand{Email: "a@b.co", Code: "123"}); err != otpdomain.ErrOTPInvalid {
		t.Fatalf("Handle() error = %v, want ErrOTPInvalid", err)
	}
}

func TestVerifyOTPConcurrentOnlyOneWins(t *testing.T) {
	store := newFakeStore()
	ctx := context.Background()
	if err := store.Set(ctx, "a@b.co", "123456", time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	h := NewVerifyOTPHandler(store)

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	results := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx] = h.Handle(ctx, VerifyOTPCommand{Email: "a@b.co", Code: "123456"})
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent successes = %d, want exactly 1", successes)
	}
}

func TestRequestOTPCooldownSkipsSecondSend(t *testing.T) {
	store := newFakeStore()
	sender := &fakeSender{}
	h := NewRequestOTPCommandHandler(store, sender, testLogger())
	for _, email := range []string{"Alice@Example.COM", "alice@example.com"} {
		if err := h.Handle(t.Context(), RequestOTPCommand{Email: email}); err != nil {
			t.Fatal(err)
		}
	}
	if sender.sent != 1 || store.setN != 1 {
		t.Fatalf("sent = %d, stored = %d; want one issuance during cooldown", sender.sent, store.setN)
	}
}

func TestRequestOTPStoreFailureAllowsRetry(t *testing.T) {
	store := newFakeStore()
	store.setE = errors.New("redis unavailable")
	sender := &fakeSender{}
	h := NewRequestOTPCommandHandler(store, sender, testLogger())
	cmd := RequestOTPCommand{Email: "a@b.co"}
	if err := h.Handle(t.Context(), cmd); !errors.Is(err, store.setE) {
		t.Fatalf("Handle() error = %v", err)
	}
	if store.cooldownActive(cooldownKey(cmd.Email)) {
		t.Fatal("failed storage must release cooldown")
	}
	if _, err := store.Get(t.Context(), cmd.Email); !errors.Is(err, otpdomain.ErrOTPExpired) {
		t.Fatalf("Get() error = %v", err)
	}
	store.setE = nil
	if err := h.Handle(t.Context(), cmd); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if sender.sent != 2 || store.setN != 1 {
		t.Fatalf("sent = %d, stored = %d", sender.sent, store.setN)
	}
}

type inspectingSender func(context.Context, maildomain.MailMessage) error

func (s inspectingSender) Send(ctx context.Context, msg maildomain.MailMessage) error {
	return s(ctx, msg)
}

type ttlStore struct {
	*fakeStore
	ttl time.Duration
}

func (s *ttlStore) Set(ctx context.Context, email, code string, ttl time.Duration) error {
	s.ttl = ttl
	return s.fakeStore.Set(ctx, email, code, ttl)
}

func TestRequestOTPStartsFullTTLAfterSend(t *testing.T) {
	store := &ttlStore{fakeStore: newFakeStore()}
	sender := inspectingSender(func(ctx context.Context, msg maildomain.MailMessage) error {
		if _, err := store.Get(ctx, msg.To); !errors.Is(err, otpdomain.ErrOTPExpired) {
			t.Fatalf("OTP should not be active during send: %v", err)
		}
		if store.setN != 0 {
			t.Fatal("OTP persisted before mail submission")
		}
		return nil
	})
	h := NewRequestOTPCommandHandler(store, sender, testLogger())
	if err := h.Handle(t.Context(), RequestOTPCommand{Email: "a@b.co"}); err != nil {
		t.Fatal(err)
	}
	if store.ttl != otpdomain.TTL || store.setN != 1 {
		t.Fatalf("TTL = %v, writes = %d", store.ttl, store.setN)
	}
}

func TestVerifyOTPNormalizesEmail(t *testing.T) {
	store := newFakeStore()
	if err := store.Set(t.Context(), "alice@example.com", "123456", time.Minute); err != nil {
		t.Fatal(err)
	}
	h := NewVerifyOTPHandler(store)
	if err := h.Handle(t.Context(), VerifyOTPCommand{Email: "Alice@Example.COM", Code: "123456"}); err != nil {
		t.Fatal(err)
	}
}

func TestRequestOTPResetsPreviousAttempts(t *testing.T) {
	store := newFakeStore()
	email := "a@b.co"
	store.attempts[attemptsKey(email)] = maxVerifyAttempts
	h := NewRequestOTPCommandHandler(store, &fakeSender{}, testLogger())
	if err := h.Handle(t.Context(), RequestOTPCommand{Email: email}); err != nil {
		t.Fatal(err)
	}
	if got := store.attemptCount(attemptsKey(email)); got != 0 {
		t.Fatalf("attempts = %d, want 0", got)
	}
}
