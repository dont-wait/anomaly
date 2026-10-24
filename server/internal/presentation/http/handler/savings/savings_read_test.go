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
