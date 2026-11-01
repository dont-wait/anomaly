package savings

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/application/account/queries"
	savingsqueries "github.com/dont-wait/anomaly/internal/application/savings/queries"
	"github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/domain/auth"
	"github.com/dont-wait/anomaly/internal/domain/savings"
	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
)

type savingsTermsStub struct {
	snapshot TermsSnapshot
	err      error
}

func (s savingsTermsStub) Current(context.Context) (TermsSnapshot, error) {
	return s.snapshot, s.err
}

type savingsTokenStub struct{ queries.TokenService }

func (savingsTokenStub) Parse(string) (*auth.Claims, error) {
	return &auth.Claims{UserID: "owner", Role: "user"}, nil
}

type httpCatalog struct{ items []savingsqueries.Package }

func (c httpCatalog) ListActive(context.Context) ([]savingsqueries.Package, error) {
	return c.items, nil
}

type httpAccounts struct{}

func (httpAccounts) FindByID(_ context.Context, id string) (*account.UserAccount, error) {
	if id == "owner" {
		return &account.UserAccount{
			Id: "owner", CustomerId: "customer", Status: account.AccountStatusActive,
			Type: account.AccountTypePayment, Currency: account.CurrencyVND,
			Balance: account.Balance{Current: 5_000_000},
		}, nil
	}
	return nil, nil
}

type httpQuotes struct{ saved savingsqueries.Quote }

func (h *httpQuotes) Save(_ context.Context, q savingsqueries.Quote) error { h.saved = q; return nil }

func (h *httpQuotes) Get(_ context.Context, id string) (savingsqueries.Quote, bool, error) {
	return h.saved, id == h.saved.ID, nil
}

func newTestRouter() (*http.ServeMux, *httpQuotes) {
	terms := make([]savingsqueries.TermOption, 0)
	for _, t := range savings.CurrentTermRates() {
		var label string
		if t.Months%12 == 0 {
			label = map[int]string{12: "1 năm", 24: "2 năm", 36: "3 năm", 60: "5 năm"}[t.Months]
		} else {
			label = map[int]string{3: "3 tháng", 6: "6 tháng", 9: "9 tháng"}[t.Months]
		}
		terms = append(terms, savingsqueries.TermOption{TermMonths: t.Months, TermLabel: label, AnnualInterestRate: savingsqueries.RateString(t.AnnualRateBps)})
	}
	store := &httpQuotes{}
	h := &Handler{Terms: savingsTermsStub{snapshot: TermsSnapshot{
		Version:      "db-terms-v2",
		Title:        "Điều kiện gửi tiết kiệm",
		Introduction: "Thông tin sản phẩm từ MongoDB.",
		Sections:     []TermsSection{{Title: "Đáo hạn", Body: "Tất toán theo kỳ hạn đã chọn."}},
	}}, Service: &savingsqueries.Service{
		Catalog: httpCatalog{items: []savingsqueries.Package{{
			PackageID: savingsqueries.StandardPackageID, Name: "Tiết kiệm có kỳ hạn", Currency: "VND",
			MinimumDeposit: savings.MinimumDepositVND, EffectiveAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), TermOptions: terms,
		}}}, Accounts: httpAccounts{}, Quotes: store,
		Now: func() time.Time { return time.Date(2026, 10, 12, 2, 10, 0, 0, time.UTC) },
	}}
	mux := http.NewServeMux()
	r := openapi.NewRegistry(mux, false)
	RegisterRoutes(r, h, savingsTokenStub{})
	return mux, store
}

func TestSavingsRoutesRequireJWT(t *testing.T) {
	mux, _ := newTestRouter()
	for _, tt := range []struct{ method, path string }{{"GET", "/api/savings/packages"}, {"GET", "/api/savings/terms"}, {"POST", "/api/savings/quotes"}} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s returned %d instead of 401", tt.method, tt.path, w.Code)
		}
	}
}

func TestSavingsRoutesPackagesAndQuote(t *testing.T) {
	mux, store := newTestRouter()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/savings/packages", nil)
	r.Header.Set("Authorization", "Bearer valid")
	mux.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("packages HTTP status=%d body=%s", w.Code, w.Body.String())
	}
	var packages struct {
		Items []savingsqueries.Package `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &packages); err != nil || len(packages.Items) != 1 || len(packages.Items[0].TermOptions) != 7 {
		t.Fatalf("unexpected packages response: %+v error=%v", packages, err)
	}

	body := []byte(`{"packageId":"savings_standard","sourceAccountId":"owner","amount":1000000,"termMonths":3}`)
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/savings/quotes", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer valid")
	mux.ServeHTTP(w, r)
	if w.Code != 200 || store.saved.ID == "" || store.saved.ActorID != "owner" {
		t.Fatalf("quote status=%d body=%s saved=%+v", w.Code, w.Body.String(), store.saved)
	}
}

func TestSavingsQuoteRejectsBadJSON(t *testing.T) {
	mux, _ := newTestRouter()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/savings/quotes", bytes.NewBufferString(`{"unknown":5}`))
	r.Header.Set("Authorization", "Bearer valid")
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestSavingsTermsAreVersionedAndDoNotDependOnBankWrites(t *testing.T) {
	mux, _ := newTestRouter()
	w := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/savings/terms", nil)
	request.Header.Set("Authorization", "Bearer valid")
	mux.ServeHTTP(w, request)
	if w.Code != http.StatusOK {
		t.Fatalf("terms status=%d body=%s", w.Code, w.Body.String())
	}
	var terms TermsSnapshot
	if err := json.Unmarshal(w.Body.Bytes(), &terms); err != nil {
		t.Fatal(err)
	}
	if terms.Version != "db-terms-v2" || len(terms.Sections) != 1 || terms.Introduction == "" {
		t.Fatalf("incomplete savings product conditions: %+v", terms)
	}
	// Customer-facing wording must not contain engineering-scope disclaimers.
	if strings.Contains(strings.ToLower(w.Body.String()), "mvp") || strings.Contains(w.Body.String(), "chưa phải") {
		t.Fatalf("product conditions expose internal engineering text: %s", w.Body.String())
	}
	// The endpoint is read-only and available without command/write flags.
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, request)
	if w.Code != http.StatusOK {
		t.Fatalf("terms follow-up status=%d", w.Code)
	}
}

func TestSavingsTermsAreUnavailableWithoutPublishedDocument(t *testing.T) {
	mux := http.NewServeMux()
	reg := openapi.NewRegistry(mux, false)
	RegisterRoutes(reg, &Handler{}, savingsTokenStub{})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/savings/terms", nil)
	r.Header.Set("Authorization", "Bearer valid")
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "SAVINGS_TERMS_UNAVAILABLE") {
		t.Fatalf("missing terms must fail closed: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestSavingsTermsAreReadFreshForEachRequest(t *testing.T) {
	// Mutate the injected store to prove the handler never caches a hardcoded snapshot.
	store := &mutableSavingsTermsStore{snapshot: TermsSnapshot{
		Version: "terms-v1", Title: "Điều kiện", Introduction: "Nội dung từ DB",
		Sections: []TermsSection{{Title: "Kỳ hạn", Body: "Đáo hạn theo hợp đồng"}},
	}}
	mux := http.NewServeMux()
	reg := openapi.NewRegistry(mux, false)
	RegisterRoutes(reg, &Handler{Terms: store}, savingsTokenStub{})
	request := httptest.NewRequest(http.MethodGet, "/api/savings/terms", nil)
	request.Header.Set("Authorization", "Bearer valid")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, request)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "terms-v1") {
		t.Fatalf("first response: %d %s", w.Code, w.Body.String())
	}
	store.snapshot.Version = "terms-v2"
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, request)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "terms-v2") {
		t.Fatalf("updated response: %d %s", w.Code, w.Body.String())
	}
}

type mutableSavingsTermsStore struct{ snapshot TermsSnapshot }

func (s *mutableSavingsTermsStore) Current(context.Context) (TermsSnapshot, error) {
	return s.snapshot, nil
}
