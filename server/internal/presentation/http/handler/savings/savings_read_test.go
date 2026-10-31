package savings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
)

func TestSavingsReadEndpointsInOpenAPI(t *testing.T) {
	reg := openapi.NewRegistry(http.NewServeMux(), true)
	RegisterRoutes(reg, &Handler{}, savingsTokenStub{})
	w := httptest.NewRecorder()
	reg.SpecHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected spec status %d", w.Code)
	}
	var spec struct {
		Paths map[string]map[string]struct {
			Responses map[string]any `json:"responses"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/api/savings/contracts",
		"/api/savings/contracts/{id}",
		"/api/savings/contracts/{id}/history",
		"/api/savings/notifications",
	} {
		if _, ok := spec.Paths[path]["get"]; !ok {
			t.Errorf("missing GET %s", path)
		}
	}
}

func TestSavingsReadOpenAPIQueryAndNotFoundResponses(t *testing.T) {
	reg := openapi.NewRegistry(http.NewServeMux(), true)
	RegisterRoutes(reg, &Handler{}, savingsTokenStub{})
	w := httptest.NewRecorder()
	reg.SpecHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	var spec struct {
		Paths map[string]map[string]struct {
			Responses  map[string]any `json:"responses"`
			Parameters []struct {
				Name     string `json:"name"`
				In       string `json:"in"`
				Required bool   `json:"required"`
				Schema   struct {
					Type string `json:"type"`
				} `json:"schema"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path   string
		params map[string]string
	}{
		{"/api/savings/contracts", map[string]string{"status": "string", "cursor": "string", "limit": "integer"}},
		{"/api/savings/notifications", map[string]string{"limit": "integer"}},
	} {
		op, ok := spec.Paths[tc.path]["get"]
		if !ok {
			t.Fatalf("missing GET %s", tc.path)
		}
		if _, ok := op.Responses["404"]; !ok {
			t.Errorf("%s missing 404 response", tc.path)
		}
		if len(op.Parameters) != len(tc.params) {
			t.Errorf("%s declared %d query parameters, want %d", tc.path, len(op.Parameters), len(tc.params))
		}
		for _, p := range op.Parameters {
			want, exists := tc.params[p.Name]
			if !exists || p.In != "query" || p.Required || p.Schema.Type != want {
				t.Errorf("%s invalid query parameter %+v (type want %q)", tc.path, p, want)
			}
		}
	}
}
