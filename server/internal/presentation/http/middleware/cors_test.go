package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewCORSAllowsIdempotencyKeyPreflight(t *testing.T) {
	handler := NewCORS([]string{"http://localhost:1420"})(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("preflight request reached the next handler")
		}),
	)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/api/transfers", nil)
	request.Header.Set("Origin", "http://localhost:1420")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "idempotency-key")

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if recorder.Header().Get("Access-Control-Allow-Origin") != "http://localhost:1420" {
		t.Fatalf("allow origin = %q", recorder.Header().Get("Access-Control-Allow-Origin"))
	}
	if headers := strings.ToLower(recorder.Header().Get("Access-Control-Allow-Headers")); !strings.Contains(headers, "idempotency-key") {
		t.Fatalf("allow headers = %q, want Idempotency-Key", headers)
	}
}
