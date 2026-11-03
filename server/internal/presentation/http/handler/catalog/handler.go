// Package catalog là API admin quản lý gói và tra cứu hợp đồng.
package catalog

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	accountqueries "github.com/dont-wait/anomaly/internal/application/account/queries"
	appcatalog "github.com/dont-wait/anomaly/internal/application/catalog"
	loanqueries "github.com/dont-wait/anomaly/internal/application/loan/queries"
	savingsqueries "github.com/dont-wait/anomaly/internal/application/savings/queries"
	domain "github.com/dont-wait/anomaly/internal/domain/catalog"
	"github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type LoanContracts interface {
	List(context.Context, mongo.AdminLoanContractFilter, int, string) (mongo.AdminLoanContractPage, error)
	Detail(context.Context, string) (mongo.AdminLoanContractDetail, error)
}

type Handler struct {
	Catalog *appcatalog.Service
	Savings *savingsqueries.AdminSavingsService
	Loans   LoanContracts
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, appcatalog.ErrPackageNotFound),
		errors.Is(err, appcatalog.ErrTermsNotFound),
		errors.Is(err, loanqueries.ErrLoanContractNotFound),
		errors.Is(err, savingsqueries.ErrContractNotFound):
		return http.StatusNotFound
	case errors.Is(err, appcatalog.ErrDraftExists),
		errors.Is(err, appcatalog.ErrNoDraft),
		errors.Is(err, appcatalog.ErrVersionConflict),
		errors.Is(err, appcatalog.ErrCodeExists),
		errors.Is(err, appcatalog.ErrPackageInUse),
		errors.Is(err, appcatalog.ErrPackageActive):
		return http.StatusConflict
	case errors.Is(err, domain.ErrInvalidPackage):
		return http.StatusUnprocessableEntity
	case errors.Is(err, appcatalog.ErrInvalidRequest),
		errors.Is(err, savingsqueries.ErrInvalidRead),
		errors.Is(err, mongo.ErrInvalidAdminLoanCursor):
		return http.StatusBadRequest
	case errors.Is(err, loanqueries.ErrLoanContractNotReady),
		errors.Is(err, savingsqueries.ErrReadNotReady):
		return http.StatusServiceUnavailable
	}
	return http.StatusInternalServerError
}

func codeFor(err error) httpx.ErrorCode {
	switch {
	case errors.Is(err, appcatalog.ErrPackageNotFound):
		return "PACKAGE_NOT_FOUND"
	case errors.Is(err, appcatalog.ErrDraftExists):
		return "DRAFT_ALREADY_EXISTS"
	case errors.Is(err, appcatalog.ErrNoDraft):
		return "NO_DRAFT"
	case errors.Is(err, appcatalog.ErrVersionConflict):
		return "PACKAGE_VERSION_CONFLICT"
	case errors.Is(err, appcatalog.ErrCodeExists):
		return "PACKAGE_CODE_EXISTS"
	case errors.Is(err, appcatalog.ErrPackageInUse):
		return "PACKAGE_IN_USE"
	case errors.Is(err, appcatalog.ErrPackageActive):
		return "PACKAGE_ACTIVE"
	case errors.Is(err, domain.ErrInvalidPackage):
		return "INVALID_PACKAGE"
	}
	return httpx.ErrorCodeForStatus(statusFor(err))
}

func failWith(w http.ResponseWriter, err error) {
	httpx.WriteError(w, log.Logger, err, statusFor, codeFor)
}

func actorOf(r *http.Request) string {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		return ""
	}
	return claims.UserID
}

func idempotencyKey(r *http.Request) (string, error) {
	key := r.Header.Get("Idempotency-Key")
	if _, err := uuid.Parse(key); err != nil {
		return "", errors.Join(appcatalog.ErrInvalidRequest, errors.New("Idempotency-Key must be a UUID"))
	}
	return key, nil
}

func limitOf(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 20, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.Join(appcatalog.ErrInvalidRequest, errors.New("invalid limit"))
	}
	return limit, nil
}

func (h *Handler) list(kind domain.Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := h.Catalog.List(r.Context(), kind, r.URL.Query().Get("status"), r.URL.Query().Get("q"))
		if err != nil {
			failWith(w, err)
			return
		}
		httpx.WriteSuccess(w, 200, "packages", map[string]any{"items": items})
	}
}

func (h *Handler) get(kind domain.Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		detail, err := h.Catalog.Get(r.Context(), kind, r.PathValue("code"))
		if err != nil {
			failWith(w, err)
			return
		}
		httpx.WriteSuccess(w, 200, "package", detail)
	}
}

func (h *Handler) create(kind domain.Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in appcatalog.CreateInput
		if err := httpx.DecodeJSON(w, r, &in); err != nil {
			failWith(w, errors.Join(appcatalog.ErrInvalidRequest, errors.New("invalid package JSON")))
			return
		}
		created, err := h.Catalog.CreatePackage(r.Context(), kind, in, actorOf(r))
		if err != nil {
			failWith(w, err)
			return
		}
		httpx.WriteSuccess(w, 201, "package created", created)
	}
}

func (h *Handler) remove(kind domain.Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h.Catalog.DeletePackage(r.Context(), kind, r.PathValue("code"), actorOf(r)); err != nil {
			failWith(w, err)
			return
		}
		httpx.WriteSuccess(w, 200, "package deleted", nil)
	}
}

func (h *Handler) createDraft(kind domain.Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		draft, err := h.Catalog.CreateDraft(r.Context(), kind, r.PathValue("code"), actorOf(r))
		if err != nil {
			failWith(w, err)
			return
		}
		httpx.WriteSuccess(w, 201, "draft created", draft)
	}
}

func (h *Handler) updateDraft(kind domain.Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in appcatalog.DraftInput
		if err := httpx.DecodeJSON(w, r, &in); err != nil {
			failWith(w, errors.Join(appcatalog.ErrInvalidRequest, errors.New("invalid draft JSON")))
			return
		}
		draft, err := h.Catalog.UpdateDraft(r.Context(), kind, r.PathValue("code"), in)
		if err != nil {
			failWith(w, err)
			return
		}
		httpx.WriteSuccess(w, 200, "draft saved", draft)
	}
}

func (h *Handler) deleteDraft(kind domain.Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h.Catalog.DeleteDraft(r.Context(), kind, r.PathValue("code")); err != nil {
			failWith(w, err)
			return
		}
		httpx.WriteSuccess(w, 200, "draft deleted", nil)
	}
}

type transitionRequest struct {
	ExpectedVersion int    `json:"expectedVersion"`
	Reason          string `json:"reason,omitempty"`
}

func (h *Handler) publish(kind domain.Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key, err := idempotencyKey(r)
		if err != nil {
			failWith(w, err)
			return
		}
		var in transitionRequest
		if err := httpx.DecodeJSON(w, r, &in); err != nil {
			failWith(w, errors.Join(appcatalog.ErrInvalidRequest, errors.New("invalid publish JSON")))
			return
		}
		published, err := h.Catalog.Publish(r.Context(), kind, r.PathValue("code"), actorOf(r), key, in.ExpectedVersion)
		if err != nil {
			failWith(w, err)
			return
		}
		httpx.WriteSuccess(w, 200, "package published", published)
	}
}

func (h *Handler) retire(kind domain.Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key, err := idempotencyKey(r)
		if err != nil {
			failWith(w, err)
			return
		}
		var in transitionRequest
		if err := httpx.DecodeJSON(w, r, &in); err != nil {
			failWith(w, errors.Join(appcatalog.ErrInvalidRequest, errors.New("invalid retire JSON")))
			return
		}
		retired, err := h.Catalog.Retire(r.Context(), kind, r.PathValue("code"), actorOf(r), key, in.Reason, in.ExpectedVersion)
		if err != nil {
			failWith(w, err)
			return
		}
		httpx.WriteSuccess(w, 200, "package retired", retired)
	}
}

type loanSimulationRequest struct {
	Amount        int64  `json:"amount"`
	MonthlyIncome int64  `json:"monthlyIncome"`
	RegionCode    string `json:"regionCode"`
	TermMonths    int    `json:"termMonths"`
}

func (h *Handler) simulateLoan(w http.ResponseWriter, r *http.Request) {
	var in loanSimulationRequest
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		failWith(w, errors.Join(appcatalog.ErrInvalidRequest, errors.New("invalid simulation JSON")))
		return
	}
	result, err := h.Catalog.SimulateLoan(in.Amount, in.MonthlyIncome, in.RegionCode, in.TermMonths)
	if err != nil {
		failWith(w, err)
		return
	}
	httpx.WriteSuccess(w, 200, "simulation", result)
}

type savingsSimulationRequest struct {
	Amount        int64 `json:"amount"`
	AnnualRateBps int64 `json:"annualRateBps"`
	TermMonths    int   `json:"termMonths"`
}

func (h *Handler) simulateSavings(w http.ResponseWriter, r *http.Request) {
	var in savingsSimulationRequest
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		failWith(w, errors.Join(appcatalog.ErrInvalidRequest, errors.New("invalid simulation JSON")))
		return
	}
	result, err := h.Catalog.SimulateSavings(in.Amount, in.AnnualRateBps, in.TermMonths)
	if err != nil {
		failWith(w, err)
		return
	}
	httpx.WriteSuccess(w, 200, "simulation", result)
}

func (h *Handler) listTerms(w http.ResponseWriter, r *http.Request) {
	items, err := h.Catalog.ListTerms(r.Context())
	if err != nil {
		failWith(w, err)
		return
	}
	httpx.WriteSuccess(w, 200, "savings terms", map[string]any{"items": items})
}

func (h *Handler) saveTermsDraft(w http.ResponseWriter, r *http.Request) {
	var in appcatalog.TermsDocument
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		failWith(w, errors.Join(appcatalog.ErrInvalidRequest, errors.New("invalid terms JSON")))
		return
	}
	draft, err := h.Catalog.SaveTermsDraft(r.Context(), in, actorOf(r))
	if err != nil {
		failWith(w, err)
		return
	}
	httpx.WriteSuccess(w, 200, "terms draft saved", draft)
}

func (h *Handler) deleteTermsDraft(w http.ResponseWriter, r *http.Request) {
	if err := h.Catalog.DeleteTermsDraft(r.Context()); err != nil {
		failWith(w, err)
		return
	}
	httpx.WriteSuccess(w, 200, "terms draft deleted", nil)
}

type publishTermsRequest struct {
	Version string `json:"version"`
}

func (h *Handler) publishTerms(w http.ResponseWriter, r *http.Request) {
	var in publishTermsRequest
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		failWith(w, errors.Join(appcatalog.ErrInvalidRequest, errors.New("invalid terms JSON")))
		return
	}
	published, err := h.Catalog.PublishTerms(r.Context(), in.Version, actorOf(r))
	if err != nil {
		failWith(w, err)
		return
	}
	httpx.WriteSuccess(w, 200, "terms published", published)
}

func parseDate(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	at, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, errors.Join(appcatalog.ErrInvalidRequest, errors.New("dates must be YYYY-MM-DD"))
	}
	return at, nil
}

func (h *Handler) listSavingsContracts(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit, err := limitOf(r)
	if err != nil {
		failWith(w, err)
		return
	}
	from, err := parseDate(query.Get("from"))
	if err != nil {
		failWith(w, err)
		return
	}
	to, err := parseDate(query.Get("to"))
	if err != nil {
		failWith(w, err)
		return
	}
	if !to.IsZero() {
		to = to.AddDate(0, 0, 1).Add(-time.Nanosecond)
	}
	page, err := h.Savings.List(r.Context(), savingsqueries.AdminSavingsFilter{
		Status: query.Get("status"), PackageID: query.Get("packageId"), CustomerID: query.Get("customerId"),
		Query: query.Get("q"), From: from, To: to,
	}, limit, query.Get("cursor"))
	if err != nil {
		failWith(w, err)
		return
	}
	httpx.WriteSuccess(w, 200, "savings contracts", page)
}

func (h *Handler) savingsContract(w http.ResponseWriter, r *http.Request) {
	detail, err := h.Savings.Detail(r.Context(), r.PathValue("id"))
	if err != nil {
		failWith(w, err)
		return
	}
	httpx.WriteSuccess(w, 200, "savings contract", detail)
}

func (h *Handler) listLoanContracts(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit, err := limitOf(r)
	if err != nil {
		failWith(w, err)
		return
	}
	page, err := h.Loans.List(r.Context(), mongo.AdminLoanContractFilter{
		Status: query.Get("status"), PackageCode: query.Get("packageCode"), CustomerID: query.Get("customerId"),
		Overdue: query.Get("overdue") == "true",
	}, limit, query.Get("cursor"))
	if err != nil {
		failWith(w, err)
		return
	}
	httpx.WriteSuccess(w, 200, "loan contracts", page)
}

func (h *Handler) loanContract(w http.ResponseWriter, r *http.Request) {
	detail, err := h.Loans.Detail(r.Context(), r.PathValue("id"))
	if err != nil {
		failWith(w, err)
		return
	}
	httpx.WriteSuccess(w, 200, "loan contract", detail)
}

// RegisterRoutes đăng ký toàn bộ route, đều yêu cầu admin hiện hành.
func RegisterRoutes(router *openapi.Registry, h *Handler, tokens accountqueries.TokenService, currentAdmin func(http.Handler) http.Handler) {
	if router == nil || h == nil || tokens == nil || currentAdmin == nil {
		return
	}
	failures := []int{400, 401, 403, 404, 409, 422, 500, 503}
	add := func(pattern string, fn http.HandlerFunc, op openapi.Operation) {
		op.Auth, op.Tags, op.FailureStatuses = true, []string{"Admin Products"}, failures
		router.Handle(pattern, middleware.RequireAuth(tokens)(currentAdmin(fn)), op)
	}
	idempotency := []openapi.Parameter{{Name: "Idempotency-Key", In: "header", Required: true, Type: "string", Format: "uuid"}}
	for _, kind := range []struct {
		value domain.Kind
		path  string
		id    string
	}{{domain.KindLoan, "loan-packages", "LoanPackage"}, {domain.KindSavings, "savings-packages", "SavingsPackage"}} {
		root := "/api/admin/" + kind.path
		add("GET "+root, h.list(kind.value), openapi.Operation{ID: "adminList" + kind.id + "s", Summary: "List packages"})
		add("POST "+root, h.create(kind.value), openapi.Operation{ID: "adminCreate" + kind.id, Summary: "Create a new package as a draft", SuccessStatus: http.StatusCreated, Request: appcatalog.CreateInput{}, Response: domain.Package{}})
		add("DELETE "+root+"/{code}", h.remove(kind.value), openapi.Operation{ID: "adminDelete" + kind.id, Summary: "Delete a package that is not on sale and was never used"})
		add("GET "+root+"/{code}", h.get(kind.value), openapi.Operation{ID: "adminGet" + kind.id, Summary: "Read a package with versions, usage and audit", Response: appcatalog.Detail{}})
		add("POST "+root+"/{code}/draft", h.createDraft(kind.value), openapi.Operation{ID: "adminCreate" + kind.id + "Draft", Summary: "Copy the latest version into a draft", SuccessStatus: http.StatusCreated, Response: domain.Package{}})
		add("PATCH "+root+"/{code}/draft", h.updateDraft(kind.value), openapi.Operation{ID: "adminUpdate" + kind.id + "Draft", Summary: "Edit the draft", Request: appcatalog.DraftInput{}, Response: domain.Package{}})
		add("DELETE "+root+"/{code}/draft", h.deleteDraft(kind.value), openapi.Operation{ID: "adminDelete" + kind.id + "Draft", Summary: "Delete the draft"})
		add("POST "+root+"/{code}/publish", h.publish(kind.value), openapi.Operation{ID: "adminPublish" + kind.id, Summary: "Publish the draft; existing contracts are unaffected", Parameters: idempotency, Request: transitionRequest{}, Response: domain.Package{}})
		add("POST "+root+"/{code}/retire", h.retire(kind.value), openapi.Operation{ID: "adminRetire" + kind.id, Summary: "Stop new sales; existing contracts are unaffected", Parameters: idempotency, Request: transitionRequest{}, Response: domain.Package{}})
	}
	add("POST /api/admin/loan-simulations", h.simulateLoan, openapi.Operation{ID: "adminSimulateLoan", Summary: "Simulate a loan with the current rate policy", Request: loanSimulationRequest{}, Response: appcatalog.LoanSimulation{}})
	add("POST /api/admin/savings-simulations", h.simulateSavings, openapi.Operation{ID: "adminSimulateSavings", Summary: "Simulate maturity interest", Request: savingsSimulationRequest{}, Response: appcatalog.SavingsSimulation{}})
	add("GET /api/admin/savings-terms", h.listTerms, openapi.Operation{ID: "adminListSavingsTerms", Summary: "List savings terms versions"})
	add("PUT /api/admin/savings-terms/draft", h.saveTermsDraft, openapi.Operation{ID: "adminSaveSavingsTermsDraft", Summary: "Create or update the terms draft", Request: appcatalog.TermsDocument{}, Response: appcatalog.TermsDocument{}})
	add("DELETE /api/admin/savings-terms/draft", h.deleteTermsDraft, openapi.Operation{ID: "adminDeleteSavingsTermsDraft", Summary: "Delete the terms draft"})
	add("POST /api/admin/savings-terms/publish", h.publishTerms, openapi.Operation{ID: "adminPublishSavingsTerms", Summary: "Publish the terms draft", Request: publishTermsRequest{}, Response: appcatalog.TermsDocument{}})
	add("GET /api/admin/savings/contracts", h.listSavingsContracts, openapi.Operation{ID: "adminListSavingsContracts", Summary: "List savings contracts of all customers", Response: savingsqueries.AdminSavingsPage{}})
	add("GET /api/admin/savings/contracts/{id}", h.savingsContract, openapi.Operation{ID: "adminGetSavingsContract", Summary: "Read a savings contract with history", Response: savingsqueries.AdminSavingsDetail{}})
	add("GET /api/admin/loans/contracts", h.listLoanContracts, openapi.Operation{ID: "adminListLoanContracts", Summary: "List loan contracts of all customers", Response: mongo.AdminLoanContractPage{}})
	add("GET /api/admin/loans/contracts/{id}", h.loanContract, openapi.Operation{ID: "adminGetLoanContract", Summary: "Read a loan contract with schedule and payments", Response: mongo.AdminLoanContractDetail{}})
}
