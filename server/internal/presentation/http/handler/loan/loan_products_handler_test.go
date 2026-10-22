package loan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	loanqueries "github.com/dont-wait/anomaly/internal/application/loan/queries"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
)

type httpLoanProductReader struct {
	items []loanqueries.LoanPackage
	err   error
}

func (r httpLoanProductReader) ListActiveLoanPackages(context.Context) ([]loanqueries.LoanPackage, error) {
	return r.items, r.err
}

func testProductHandler(reader httpLoanProductReader) *Handler {
	return &Handler{
		Products:            loanqueries.NewLoanProductCatalogQueryService(reader),
		PreApplicationQuote: loanqueries.NewPreApplicationQuoteQueryService(reader),
	}
}

func TestLoanProductsHTTP(t *testing.T) {
	p := loanqueries.LoanPackage{PackageCode: "PERSONAL_UNSECURED_SALARY", Name: "Salary loan", MinAmount: 1_000_000, MaxAmount: 500_000_000, TermMonths: []int{6, 12, 24, 36, 48, 60}}
	h := testProductHandler(httpLoanProductReader{items: []loanqueries.LoanPackage{p}})
	token := downloadTokenStub{}
	mux := http.NewServeMux()
	mux.Handle("GET /api/loans/packages", middleware.RequireAuth(token)(http.HandlerFunc(h.listLoanPackages)))
	mux.Handle("POST /api/loans/quotes", middleware.RequireAuth(token)(http.HandlerFunc(h.createPreApplicationQuote)))

	unauth := httptest.NewRecorder()
	mux.ServeHTTP(unauth, httptest.NewRequest("GET", "/api/loans/packages", nil))
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d", unauth.Code)
	}

	req := httptest.NewRequest("GET", "/api/loans/packages", nil)
	req.Header.Set("Authorization", "Bearer test")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("packages status=%d body=%s", w.Code, w.Body.String())
	}
	var packages struct {
		Items []loanqueries.LoanPackage `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &packages); err != nil || len(packages.Items) != 1 {
		t.Fatalf("packages=%+v err=%v", packages, err)
	}

	body := []byte(`{"loanPackageId":"PERSONAL_UNSECURED_SALARY","requestedAmount":50000000,"termMonths":12,"monthlyIncome":15000000,"regionCode":"KV01","existingMonthlyDebt":0}`)
	req = httptest.NewRequest("POST", "/api/loans/quotes", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("quote status=%d body=%s", w.Code, w.Body.String())
	}
	var q loanqueries.PreApplicationQuoteResult
	if err := json.Unmarshal(w.Body.Bytes(), &q); err != nil || q.AnnualRateBps != 1610 || q.Official {
		t.Fatalf("quote=%+v err=%v", q, err)
	}
}

func TestLoanProductsHTTPValidation(t *testing.T) {
	p := loanqueries.LoanPackage{PackageCode: "PERSONAL_UNSECURED_SALARY", Name: "Salary loan", MinAmount: 1_000_000, MaxAmount: 500_000_000, TermMonths: []int{12}}
	h := testProductHandler(httpLoanProductReader{items: []loanqueries.LoanPackage{p}})
	cases := []struct {
		body   string
		status int
	}{
		{`{`, 400},
		{`{"loanPackageId":"bad","requestedAmount":50000000,"termMonths":12,"monthlyIncome":15000000,"regionCode":"KV01"}`, 422},
		{`{"loanPackageId":"PERSONAL_UNSECURED_SALARY","requestedAmount":50000000,"termMonths":24,"monthlyIncome":15000000,"regionCode":"KV01"}`, 422},
	}
	for _, tc := range cases {
		req := httptest.NewRequest("POST", "/api/loans/quotes", bytes.NewBufferString(tc.body))
		w := httptest.NewRecorder()
		h.createPreApplicationQuote(w, req)
		if w.Code != tc.status {
			t.Fatalf("request=%s status=%d expected=%d", tc.body, w.Code, tc.status)
		}
	}
	unavailable := testProductHandler(httpLoanProductReader{err: errors.New("mongo offline")})
	w := httptest.NewRecorder()
	unavailable.listLoanPackages(w, httptest.NewRequest("GET", "/api/loans/packages", nil))
	if w.Code != 503 {
		t.Fatalf("catalog error status=%d", w.Code)
	}
}
