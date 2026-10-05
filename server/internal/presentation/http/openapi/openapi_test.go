package openapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSpecHandlerServesValidOpenAPIDocument(t *testing.T) {
	mux := http.NewServeMux()
	registry := NewRegistry(mux, true)
	registry.HandleFunc("GET /api/widgets/{id}", func(http.ResponseWriter, *http.Request) {}, Operation{
		ID:      "getWidget",
		Summary: "Get a widget",
		Response: struct {
			Name string `json:"name"`
		}{},
		FailureStatuses: []int{http.StatusUnauthorized},
		Auth:            true,
	})

	recorder := httptest.NewRecorder()
	registry.SpecHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want JSON content type", got)
	}

	var document struct {
		OpenAPI string                     `json:"openapi"`
		Paths   map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &document); err != nil {
		t.Fatalf("decode OpenAPI document: %v", err)
	}
	if document.OpenAPI != "3.0.3" {
		t.Fatalf("openapi = %q, want 3.0.3", document.OpenAPI)
	}
	if len(document.Paths) != 1 {
		t.Fatalf("paths = %d, want 1", len(document.Paths))
	}
}
