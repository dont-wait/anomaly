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
	mu   sync.Mutex
	data map[string]string
	setN int
	delN int
	getE error
	setE error
}

func newFakeStore() *fakeStore {
	return &fakeStore{data: map[string]string{}}
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

func (s *fakeStore) SetCooldown(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return true, nil
}

func (s *fakeStore) IncrAttempts(_ context.Context, _ string, _ time.Duration) (int64, error) {
	return 1, nil
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
