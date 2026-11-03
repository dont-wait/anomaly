package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dont-wait/anomaly/internal/application/account/queries"
	appcatalog "github.com/dont-wait/anomaly/internal/application/catalog"
	"github.com/dont-wait/anomaly/internal/domain/auth"
	domain "github.com/dont-wait/anomaly/internal/domain/catalog"
	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
)

type tokenStub struct{ queries.TokenService }

func (tokenStub) Parse(string) (*auth.Claims, error) {
	return &auth.Claims{UserID: "admin", Role: "admin"}, nil
}

type storeStub struct {
	appcatalog.Store
	publishErr error
	keys       []string
}

func (s *storeStub) Versions(context.Context, domain.Kind, string) ([]domain.Package, error) {
	return nil, nil
}

func (s *storeStub) ListAll(context.Context, domain.Kind) ([]domain.Package, error) {
	return []domain.Package{{Kind: domain.KindLoan, Code: "P", Version: 1, Status: domain.StatusActive, Name: "Vay", Terms: []domain.TermOption{{Months: 12}}}}, nil
}

func (s *storeStub) Publish(_ context.Context, t appcatalog.Transition) (domain.Package, error) {
	s.keys = append(s.keys, t.IdempotencyKey)
	return domain.Package{Code: t.Code, Version: t.ExpectedVersion, Status: domain.StatusActive}, s.publishErr
}

// currentAdmin mô phỏng RequireCurrentAdmin: role đọc từ dữ liệu, không từ JWT.
func newMux(store *storeStub, isAdmin bool) *http.ServeMux {
	mux := http.NewServeMux()
	router := openapi.NewRegistry(mux, false)
	currentAdmin := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isAdmin {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
	RegisterRoutes(router, &Handler{Catalog: &appcatalog.Service{Store: store}}, tokenStub{}, currentAdmin)
	return mux
}

func call(mux *http.ServeMux, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if _, skip := headers["no-auth"]; !skip {
		request.Header.Set("Authorization", "Bearer token")
	}
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	return response
}

func TestEveryRouteRequiresCurrentAdmin(t *testing.T) {
	routes := [][2]string{
		{"GET", "/api/admin/loan-packages"},
		{"POST", "/api/admin/savings-packages"},
		{"DELETE", "/api/admin/loan-packages/x"},
		{"GET", "/api/admin/savings-packages/x"},
		{"POST", "/api/admin/loan-packages/x/draft"},
		{"PATCH", "/api/admin/savings-packages/x/draft"},
		{"DELETE", "/api/admin/loan-packages/x/draft"},
		{"POST", "/api/admin/loan-packages/x/publish"},
		{"POST", "/api/admin/savings-packages/x/retire"},
		{"POST", "/api/admin/loan-simulations"},
		{"POST", "/api/admin/savings-simulations"},
		{"GET", "/api/admin/savings-terms"},
		{"PUT", "/api/admin/savings-terms/draft"},
		{"DELETE", "/api/admin/savings-terms/draft"},
		{"POST", "/api/admin/savings-terms/publish"},
		{"GET", "/api/admin/savings/contracts"},
		{"GET", "/api/admin/savings/contracts/x"},
		{"GET", "/api/admin/loans/contracts"},
		{"GET", "/api/admin/loans/contracts/x"},
	}
	demoted := newMux(&storeStub{}, false)
	for _, route := range routes {
		if got := call(demoted, route[0], route[1], "{}", nil).Code; got != http.StatusForbidden {
			t.Errorf("%s %s demoted admin got %d", route[0], route[1], got)
		}
		if got := call(demoted, route[0], route[1], "{}", map[string]string{"no-auth": ""}).Code; got != http.StatusUnauthorized {
			t.Errorf("%s %s anonymous got %d", route[0], route[1], got)
		}
	}
}

func TestListUsesSuccessEnvelope(t *testing.T) {
	response := call(newMux(&storeStub{}, true), "GET", "/api/admin/loan-packages", "", nil)
	var body struct {
		Status int `json:"status"`
		Data   struct {
			Items []appcatalog.Summary `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Status != 200 || len(body.Data.Items) != 1 {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
}

func TestPublishRequiresKeyAndMapsConflict(t *testing.T) {
	store := &storeStub{}
	mux := newMux(store, true)
	path := "/api/admin/loan-packages/P/publish"
	if got := call(mux, "POST", path, `{"expectedVersion":2}`, nil).Code; got != http.StatusBadRequest {
		t.Fatalf("missing key got %d", got)
	}
	headers := map[string]string{"Idempotency-Key": "58fd795e-b070-46c3-8f9e-9487063b8b27"}
	if got := call(mux, "POST", path, `{"expectedVersion":2}`, headers).Code; got != http.StatusOK {
		t.Fatalf("publish got %d", got)
	}
	if len(store.keys) != 1 || store.keys[0] != headers["Idempotency-Key"] {
		t.Fatalf("keys = %v", store.keys)
	}

	store.publishErr = appcatalog.ErrVersionConflict
	response := call(mux, "POST", path, `{"expectedVersion":2}`, headers)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "PACKAGE_VERSION_CONFLICT") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
}

func TestInternalErrorsDoNotLeak(t *testing.T) {
	store := &storeStub{publishErr: errors.New("mongo secret host down")}
	response := call(newMux(store, true), "POST", "/api/admin/loan-packages/P/publish", `{"expectedVersion":2}`,
		map[string]string{"Idempotency-Key": "58fd795e-b070-46c3-8f9e-9487063b8b27"})
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
}
