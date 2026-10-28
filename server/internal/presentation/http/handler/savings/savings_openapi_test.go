package savings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
)

func TestSavingsOpenAPIDeclaresCommandHeadersAndCreationStatus(t *testing.T) {
	reg := openapi.NewRegistry(http.NewServeMux(), true)
	RegisterRoutes(reg, &Handler{}, savingsTokenStub{})
	w := httptest.NewRecorder()
	reg.SpecHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("OpenAPI status %d", w.Code)
	}
	var spec struct {
		Paths map[string]map[string]struct {
			Responses  map[string]any `json:"responses"`
			Parameters []struct {
				Name     string `json:"name"`
				In       string `json:"in"`
				Required bool   `json:"required"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/savings/contracts", "/api/savings/contracts/{id}/withdrawals"} {
		op, ok := spec.Paths[path]["post"]
		if !ok {
			t.Fatalf("missing operation POST %s", path)
		}
		found := false
		for _, p := range op.Parameters {
			if p.Name == "Idempotency-Key" && p.In == "header" && p.Required {
				found = true
			}
		}
		if !found {
			t.Errorf("%s lacks required Idempotency-Key header", path)
		}
	}
	if _, ok := spec.Paths["/api/savings/contracts"]["post"].Responses["201"]; !ok {
		t.Fatal("opening API lacks 201 Created OpenAPI response")
	}
	// UC-11 must expose all three command/read endpoints and document the
	// business error statuses through the same OpenAPI registry as cmd/api.
	for _, tc := range []struct {
		path, method string
		statuses     []string
	}{
		{path: "/api/savings/contracts/{id}/withdrawal-quotes", method: "post", statuses: []string{"200", "400", "401", "404", "409", "422", "423", "503"}},
		{path: "/api/savings/contracts/{id}/withdrawals", method: "post", statuses: []string{"200", "400", "401", "404", "409", "410", "422", "423", "503"}},
		{path: "/api/savings/contracts/{id}/settlement", method: "get", statuses: []string{"200", "401", "404", "409", "503"}},
	} {
		op, ok := spec.Paths[tc.path][tc.method]
		if !ok {
			t.Errorf("missing UC-11 %s %s OpenAPI operation", tc.method, tc.path)
			continue
		}
		for _, status := range tc.statuses {
			if _, ok := op.Responses[status]; !ok {
				t.Errorf("%s %s lacks documented HTTP %s", tc.method, tc.path, status)
			}
		}
	}
}
