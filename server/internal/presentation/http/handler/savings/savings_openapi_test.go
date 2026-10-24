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
}
