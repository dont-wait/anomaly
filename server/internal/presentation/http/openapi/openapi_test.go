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
		SuccessResponses: map[int]any{http.StatusAccepted: struct {
			Status string `json:"status"`
		}{}},
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
	var path map[string]struct {
		Responses map[string]json.RawMessage `json:"responses"`
	}
	if err := json.Unmarshal(document.Paths["/api/widgets/{id}"], &path); err != nil {
		t.Fatalf("decode operation: %v", err)
	}
	if _, ok := path["get"].Responses["202"]; !ok {
		t.Fatal("additional success response 202 was not documented")
	}
	if _, ok := path["get"].Responses["401"]; !ok {
		t.Fatal("failure response 401 was not documented")
	}
}

func TestQueryParametersIncludeDefaultsAndBounds(t *testing.T) {
	doc := NewRegistry(http.NewServeMux(), true)
	doc.HandleFunc("GET /api/admin/profile-changes/page", func(http.ResponseWriter, *http.Request) {}, Operation{
		ID: "listProfileChangesPage",
		Parameters: []Parameter{
			{Name: "page", In: "query", Type: "integer", Default: "1", Minimum: 1},
			{Name: "pageSize", In: "query", Type: "integer", Default: "10", Minimum: 1, Maximum: 50},
			{Name: "status", In: "query", Type: "string", Default: "PENDING", Enum: []string{"PENDING", "APPROVED", "REJECTED", "ALL"}},
			{Name: "search", In: "query", Type: "string"},
			{Name: "cursor", In: "query", Type: "string"},
		},
	})
	body := httptest.NewRecorder()
	doc.SpecHandler().ServeHTTP(body, httptest.NewRequest("GET", "/openapi.json", nil))
	var specification struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name   string         `json:"name"`
				Schema map[string]any `json:"schema"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(body.Body.Bytes(), &specification); err != nil {
		t.Fatal(err)
	}
	params := specification.Paths["/api/admin/profile-changes/page"]["get"].Parameters
	if len(params) != 5 {
		t.Fatalf("query parameters: %d, want five", len(params))
	}
	if params[0].Name != "page" || params[0].Schema["default"] != float64(1) || params[0].Schema["minimum"] != float64(1) {
		t.Fatalf("page schema: %#v", params[0])
	}
	if params[1].Name != "pageSize" || params[1].Schema["maximum"] != float64(50) {
		t.Fatalf("pageSize schema: %#v", params[1])
	}
	if params[2].Name != "status" || params[2].Schema["default"] != "PENDING" {
		t.Fatalf("status schema: %#v", params[2])
	}
	allowed, ok := params[2].Schema["enum"].([]any)
	if !ok || len(allowed) != 4 {
		t.Fatalf("status values: %#v", params[2].Schema["enum"])
	}
}
