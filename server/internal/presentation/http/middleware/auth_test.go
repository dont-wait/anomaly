package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/auth"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
)

type tokenServiceStub struct {
	parse func(string) (*auth.Claims, error)
}

func (s tokenServiceStub) Issue(string, string, bool) (string, time.Time, error) {
	return "", time.Time{}, nil
}

func (s tokenServiceStub) Parse(token string) (*auth.Claims, error) {
	return s.parse(token)
}

func TestRequireAuthDistinguishesMissingHeader(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler should not be called")
	})

	RequireAuth(tokenServiceStub{parse: func(string) (*auth.Claims, error) {
		return nil, nil
	}})(next).ServeHTTP(recorder, request)

	assertUnauthorizedResponse(t, recorder, httpx.ErrorCodeMissingAuthHeader, ErrMissingAuthHeader.Error())
}

func TestRequireAuthDistinguishesInvalidToken(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer expired-token")
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler should not be called")
	})

	RequireAuth(tokenServiceStub{parse: func(string) (*auth.Claims, error) {
		return nil, errors.New("token expired")
	}})(next).ServeHTTP(recorder, request)

	assertUnauthorizedResponse(t, recorder, httpx.ErrorCodeInvalidToken, ErrInvalidToken.Error())
}

func assertUnauthorizedResponse(t *testing.T, recorder *httptest.ResponseRecorder, code httpx.ErrorCode, detail string) {
	t.Helper()
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}

	var response httpx.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != http.StatusUnauthorized {
		t.Fatalf("body status = %d, want 401", response.Status)
	}
	if len(response.Errors) != 1 || response.Errors[0].Code != code {
		t.Fatalf("errors = %#v, want code %q", response.Errors, code)
	}
	if response.Errors[0].Detail != detail {
		t.Fatalf("detail = %q, want %q", response.Errors[0].Detail, detail)
	}
}
