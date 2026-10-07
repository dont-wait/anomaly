package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewRouterSwaggerRoutesAreConditional(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		mux := NewRouter(http.NewServeMux(), nil, nil, nil, nil, nil, false)
		assertRouteStatus(t, mux, "/openapi.json", http.StatusNotFound)
		assertRouteStatus(t, mux, "/swagger/", http.StatusNotFound)
	})

	t.Run("enabled", func(t *testing.T) {
		mux := NewRouter(http.NewServeMux(), nil, nil, nil, nil, nil, true)
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
			"/api/accounts/lookup",
			"/api/transfers",
			"/api/transfers/{transferId}/confirm",
			"/api/transfers/{transferId}/otp/resend",
			"/api/transfers/recent-recipients",
			"/api/transactions",
			"/api/transactions/summary",
			"/api/transactions/{id}",
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

func TestTransactionOpenAPIContract(t *testing.T) {
	mux := NewRouter(http.NewServeMux(), nil, nil, nil, nil, nil, true)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	var document struct {
		Paths map[string]map[string]struct {
			Security   []map[string][]string `json:"security"`
			Parameters []struct {
				Name     string `json:"name"`
				In       string `json:"in"`
				Required bool   `json:"required"`
			} `json:"parameters"`
			Responses map[string]struct {
				Content map[string]struct {
					Schema struct {
						Properties map[string]struct {
							Type     string `json:"type"`
							Nullable bool   `json:"nullable"`
						} `json:"properties"`
					} `json:"schema"`
				} `json:"content"`
			} `json:"responses"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	create := document.Paths["/api/transfers"]["post"]
	if len(create.Security) != 1 {
		t.Fatal("transfer creation must document bearer auth")
	}
	foundKey := false
	for _, parameter := range create.Parameters {
		if parameter.Name == "Idempotency-Key" && parameter.In == "header" && parameter.Required {
			foundKey = true
		}
	}
	if !foundKey {
		t.Fatal("missing required Idempotency-Key header")
	}
	success := create.Responses["201"].Content["application/json"].Schema.Properties
	if _, ok := success["transferId"]; !ok {
		t.Fatal("create response must expose transferId directly")
	}
	if _, ok := success["data"]; ok {
		t.Fatal("transfer response must not document a data wrapper")
	}
	failure := create.Responses["400"].Content["application/json"].Schema.Properties
	if _, ok := failure["code"]; !ok {
		t.Fatal("business errors must document direct code")
	}
	unauthorized := create.Responses["401"].Content["application/json"].Schema.Properties
	if _, ok := unauthorized["errors"]; !ok {
		t.Fatal("401 must retain middleware error envelope")
	}
	history := document.Paths["/api/transactions"]["get"].Responses["200"].Content["application/json"].Schema.Properties
	if cursor := history["nextCursor"]; cursor.Type != "string" || !cursor.Nullable {
		t.Fatal("nextCursor must support string or null")
	}
	confirm := document.Paths["/api/transfers/{transferId}/confirm"]["post"].Responses["200"].Content["application/json"].Schema.Properties
	if _, ok := confirm["counterparty"]; !ok {
		t.Fatal("confirm must return transaction record")
	}
	// Runtime unauthorized requests must match the middleware contract even with docs enabled.
	missingAuth := httptest.NewRecorder()
	mux.ServeHTTP(missingAuth, httptest.NewRequest(http.MethodPost, "/api/transfers", nil))
	if missingAuth.Code != http.StatusUnauthorized {
		t.Fatalf("missing auth status = %d", missingAuth.Code)
	}
}
