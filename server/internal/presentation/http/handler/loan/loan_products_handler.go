package loan

import (
	"errors"
	"net/http"
	"os"

	loanqueries "github.com/dont-wait/anomaly/internal/application/loan/queries"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
	"github.com/dont-wait/anomaly/internal/seeder"
	"github.com/rs/zerolog"
)

func (h *Handler) listLoanPackages(w http.ResponseWriter, r *http.Request) {
	if h.Products == nil {
		fail(w, http.StatusServiceUnavailable, "loan products unavailable")
		return
	}
	products, err := h.Products.List(r.Context())
	if err != nil {
		fail(w, http.StatusServiceUnavailable, "loan product catalog unavailable")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, struct {
		Items []loanqueries.LoanPackage `json:"items"`
	}{Items: products})
}

func (h *Handler) seedLoanCatalog(w http.ResponseWriter, r *http.Request) {
	if err := seeder.ValidateEnvironment(os.Getenv("APP_ENV"), os.Getenv("SEED_DEMO_ENABLED")); err != nil {
		fail(w, http.StatusForbidden, "loan catalog seeding is disabled")
		return
	}
	created, err := seeder.SeedLoanCatalog(r.Context(), h.CatalogSeeder)
	if err != nil {
		log := h.Log
		if log == nil {
			log = zerolog.Ctx(r.Context())
		}
		log.Error().Err(err).Str("method", r.Method).Str("path", r.URL.Path).Msg("seed loan catalog failed")
		fail(w, http.StatusServiceUnavailable, "loan catalog seeding failed")
		return
	}
	result := "already_exists"
	if created {
		result = "created"
	}
	httpx.WriteJSON(w, http.StatusOK, struct {
		Status string `json:"status"`
	}{Status: result})
}

func (h *Handler) createPreApplicationQuote(w http.ResponseWriter, r *http.Request) {
	if h.PreApplicationQuote == nil {
		fail(w, http.StatusServiceUnavailable, "loan quote unavailable")
		return
	}

	var req loanqueries.PreApplicationQuoteInput
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		fail(w, http.StatusBadRequest, "invalid loan quote JSON")
		return
	}
	quote, err := h.PreApplicationQuote.Quote(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, loanqueries.ErrInvalidLoanQuoteInput):
			fail(w, http.StatusUnprocessableEntity, err.Error())
		case errors.Is(err, loanqueries.ErrLoanPackageUnavailable):
			fail(w, http.StatusUnprocessableEntity, "loan package is not available")
		default:
			fail(w, http.StatusServiceUnavailable, "loan quote unavailable")
		}
		return
	}
	httpx.WriteJSON(w, http.StatusOK, quote)
}
