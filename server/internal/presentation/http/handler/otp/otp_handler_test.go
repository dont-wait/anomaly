package otp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	appotp "github.com/dont-wait/anomaly/internal/application/otp"
	otpdomain "github.com/dont-wait/anomaly/internal/domain/otp"
)

type fakeStore struct {
	mu   sync.Mutex
	data map[string]string
	err  error
}

func newFakeStore() *fakeStore {
	return &fakeStore{data: map[string]string{}}
}

func (s *fakeStore) Set(_ context.Context, email, code string, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.data[email] = code
	return nil
}

func (s *fakeStore) Get(_ context.Context, email string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return "", s.err
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
	if s.err != nil {
		return s.err
	}
	delete(s.data, email)
	return nil
}

type fakeSender struct {
	err error
}

func (s *fakeSender) Send(_ context.Context, _, _, _ string) error {
	return s.err
}

func newTestHandler(store *fakeStore, sender *fakeSender) *Handler {
	log := zerolog.Nop()
	return NewHandler(
		log,
		appotp.NewRequestOTPCommandHandler(store, sender, log),
		appotp.NewVerifyOTPHandler(store),
	)
}

func doRequest(t *testing.T, h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestRequestOTPHandlerSuccess(t *testing.T) {
	h := newTestHandler(newFakeStore(), &fakeSender{})

	rec := doRequest(t, h.RequestOTP, `{"email":"alice@example.com"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["message"] != "otp sent" {
		t.Fatalf("message = %q, want %q", resp["message"], "otp sent")
	}
}

func TestRequestOTPHandlerRejectsUnknownField(t *testing.T) {
	h := newTestHandler(newFakeStore(), &fakeSender{})

	rec := doRequest(t, h.RequestOTP, `{"email":"a@b.co","unknown":1}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestRequestOTPHandlerInvalidEmail(t *testing.T) {
	h := newTestHandler(newFakeStore(), &fakeSender{})

	rec := doRequest(t, h.RequestOTP, `{"email":"not-an-email"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestRequestOTPHandlerStoreError(t *testing.T) {
	store := newFakeStore()
	store.err = errors.New("redis down")
	h := newTestHandler(store, &fakeSender{})

	rec := doRequest(t, h.RequestOTP, `{"email":"a@b.co"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "internal server error" {
		t.Fatalf("error = %q, want generic message", resp["error"])
	}
}

func TestVerifyOTPHandlerSuccess(t *testing.T) {
	store := newFakeStore()
	store.data["a@b.co"] = "123456"
	h := newTestHandler(store, &fakeSender{})

	rec := doRequest(t, h.VerifyOTP, `{"email":"a@b.co","code":"123456"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp["verified"] {
		t.Fatal("verified = false, want true")
	}
}

func TestVerifyOTPHandlerMismatch(t *testing.T) {
	store := newFakeStore()
	store.data["a@b.co"] = "123456"
	h := newTestHandler(store, &fakeSender{})

	rec := doRequest(t, h.VerifyOTP, `{"email":"a@b.co","code":"654321"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestVerifyOTPHandlerExpired(t *testing.T) {
	h := newTestHandler(newFakeStore(), &fakeSender{})

	rec := doRequest(t, h.VerifyOTP, `{"email":"a@b.co","code":"123456"}`)
	if rec.Code != http.StatusGone {
		t.Fatalf("status = %d, want 410", rec.Code)
	}
}
