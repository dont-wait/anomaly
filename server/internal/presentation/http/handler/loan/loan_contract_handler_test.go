package loan

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	loanqueries "github.com/dont-wait/anomaly/internal/application/loan/queries"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
	"github.com/google/uuid"
)

type contractHTTPReader struct {
	owner  string
	loanID string
	getErr error
}

func (s *contractHTTPReader) ListContractPage(_ context.Context, owner string, _ int, _ time.Time, _ string) ([]loanqueries.ContractListRow, bool, error) {
	s.owner = owner
	return []loanqueries.ContractListRow{}, false, nil
}

func (s *contractHTTPReader) ReadContract(_ context.Context, loanID, owner string) (loanqueries.ContractDetail, error) {
	s.loanID, s.owner = loanID, owner
	return loanqueries.ContractDetail{}, s.getErr
}

func TestLoanContractHTTPAuthorizationAndErrors(t *testing.T) {
	reader := &contractHTTPReader{getErr: loanqueries.ErrLoanContractNotFound}
	h := &Handler{Contracts: loanqueries.NewLoanContractQueryService(reader)}
	protectedList := middleware.RequireAuth(downloadTokenStub{})(http.HandlerFunc(h.listContracts))
	protectedGet := middleware.RequireAuth(downloadTokenStub{})(http.HandlerFunc(h.getContract))

	request := httptest.NewRequest("GET", "/api/loans/contracts", nil)
	w := httptest.NewRecorder()
	protectedList.ServeHTTP(w, request)
	if w.Code != 401 {
		t.Fatalf("missing JWT status=%d", w.Code)
	}

	request.Header.Set("Authorization", "Bearer token")
	w = httptest.NewRecorder()
	protectedList.ServeHTTP(w, request)
	if w.Code != 200 || reader.owner != "customer-1" {
		t.Fatalf("list owner/status=%q/%d", reader.owner, w.Code)
	}
	var page loanqueries.ContractListPage
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || page.Items == nil || !page.EventualConsistency {
		t.Fatalf("wrong response: %s err=%v", w.Body.String(), err)
	}

	request = httptest.NewRequest("GET", "/api/loans/contracts?limit=51", nil)
	request.Header.Set("Authorization", "Bearer token")
	w = httptest.NewRecorder()
	protectedList.ServeHTTP(w, request)
	if w.Code != 400 {
		t.Fatalf("invalid limit status=%d", w.Code)
	}

	id := uuid.NewString()
	request = httptest.NewRequest("GET", "/api/loans/contracts/detail/"+id, nil)
	request.SetPathValue("loanId", id)
	request.Header.Set("Authorization", "Bearer token")
	w = httptest.NewRecorder()
	protectedGet.ServeHTTP(w, request)
	if w.Code != 404 || reader.owner != "customer-1" || reader.loanID != id {
		t.Fatalf("detail owner/id/status=%q/%q/%d", reader.owner, reader.loanID, w.Code)
	}

	reader.getErr = loanqueries.ErrLoanContractNotReady
	w = httptest.NewRecorder()
	protectedGet.ServeHTTP(w, request)
	if w.Code != 503 {
		t.Fatalf("projection missing status=%d", w.Code)
	}
}

func (s *contractHTTPReader) ContractSnapshot(
	_ context.Context,
	_ string,
) (string, error) {
	return "test-snapshot-v1", nil
}
