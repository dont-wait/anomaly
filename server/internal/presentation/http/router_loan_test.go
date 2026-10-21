package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	loanhttp "github.com/dont-wait/anomaly/internal/presentation/http/handler/loan"
)

func TestLoanRoutesRegisteredAndProtected(t *testing.T) {
	mux := NewRouter(
		http.NewServeMux(),
		nil,
		nil,
		nil,
		nil,
		nil,
		true,
		&loanhttp.Handler{},
	)

	w := httptest.NewRecorder()

	mux.ServeHTTP(
		w,
		httptest.NewRequest(
			"GET",
			"/openapi.json",
			nil,
		),
	)

	var spec struct {
		Paths map[string]json.RawMessage `json:"paths"`
	}

	if err := json.Unmarshal(
		w.Body.Bytes(),
		&spec,
	); err != nil {
		t.Fatal(err)
	}

	paths := []string{
		"/api/loans",
		"/api/loans/{applicationId}",
		"/api/loans/contracts",
		"/api/loans/contracts/detail/{loanId}",
		"/api/loans/{applicationId}/documents",
		"/api/loans/{applicationId}/submit",
		"/api/loans/{applicationId}/review",
		"/api/loans/{applicationId}/evidence",
		"/api/loans/{applicationId}/assess",
		"/api/loans/{applicationId}/decision",
		"/api/loans/{applicationId}/disburse",
	}

	for _, p := range paths {
		if _, ok := spec.Paths[p]; !ok {
			t.Errorf("missing loan route: %s", p)
		}
	}

	unauth := httptest.NewRecorder()

	mux.ServeHTTP(
		unauth,
		httptest.NewRequest(
			"GET",
			"/api/loans",
			nil,
		),
	)

	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf(
			"unauthenticated access returned %d",
			unauth.Code,
		)
	}
}
