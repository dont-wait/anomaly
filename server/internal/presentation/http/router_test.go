package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewRouterSwaggerRoutesAreConditional(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		mux := NewRouter(http.NewServeMux(), nil, nil, nil, nil, false)
		assertRouteStatus(t, mux, "/openapi.json", http.StatusNotFound)
		assertRouteStatus(t, mux, "/swagger/", http.StatusNotFound)
	})

	t.Run("enabled", func(t *testing.T) {
		mux := NewRouter(http.NewServeMux(), nil, nil, nil, nil, true)
		assertRouteStatus(t, mux, "/openapi.json", http.StatusOK)
		assertRouteStatus(t, mux, "/swagger/", http.StatusOK)

		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
		var document struct {
			Paths map[string]json.RawMessage `json:"paths"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &document); err != nil {
			t.Fatalf("decode generated OpenAPI document: %v", err)
		}
		for _, path := range []string{
			"/health",
			"/api/auth/register",
			"/api/auth/login",
			"/api/auth/me",
			"/api/auth/otp/request",
			"/api/auth/otp/verify",
			"/api/accounts",
			"/api/accounts/{id}",
			"/api/accounts/by-email/{email}",
			"/api/accounts/{id}/verify",
			"/api/media/upload",
			"/api/media/download",
		} {
			if _, ok := document.Paths[path]; !ok {
				t.Errorf("generated OpenAPI document is missing path %q", path)
			}
		}
	})
}

func assertRouteStatus(t *testing.T, handler http.Handler, path string, want int) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != want {
		t.Fatalf("GET %s status = %d, want %d", path, recorder.Code, want)
	}
}
