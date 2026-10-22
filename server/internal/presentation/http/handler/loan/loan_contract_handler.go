package loan

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	loanqueries "github.com/dont-wait/anomaly/internal/application/loan/queries"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
)

func loanContractError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, loanqueries.ErrInvalidLoanContractQuery):
		fail(w, http.StatusBadRequest, "invalid loan contract request")

	case errors.Is(err, loanqueries.ErrLoanContractNotFound):
		fail(w, http.StatusNotFound, "loan contract not found")

	case errors.Is(err, loanqueries.ErrLoanContractNotReady):
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{
			"code":  "CONTRACT_PROJECTION_PENDING",
			"error": "loan contract projection is not ready",
		})

	case errors.Is(err, loanqueries.ErrLoanContractPageChanged):
		httpx.WriteJSON(w, http.StatusConflict, map[string]string{
			"code":  "CONTRACT_LIST_CHANGED",
			"error": "loan contract list changed; restart pagination",
		})

	default:
		slog.ErrorContext(
			r.Context(),
			"loan contract read failed",
			"error", err,
			"method", r.Method,
			"route", r.Pattern,
		)

		fail(
			w,
			http.StatusServiceUnavailable,
			"loan contract read service unavailable",
		)
	}
}

// GET /api/loans/contracts?limit=20&cursor=...
func (h *Handler) listContracts(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := claims(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.Contracts == nil {
		loanContractError(w, r, loanqueries.ErrLoanContractNotReady)
		return
	}
	limit := 20
	if values, exists := r.URL.Query()["limit"]; exists {
		if len(values) != 1 {
			loanContractError(w, r, loanqueries.ErrInvalidLoanContractQuery)
			return
		}
		value, err := strconv.Atoi(values[0])
		if err != nil || value < 1 || value > 50 {
			loanContractError(w, r, loanqueries.ErrInvalidLoanContractQuery)
			return
		}
		limit = value
	}
	page, err := h.Contracts.List(r.Context(), actor, limit, r.URL.Query().Get("cursor"))
	if err != nil {
		loanContractError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, page)
}

// GET /api/loans/contracts/detail/{loanId}
func (h *Handler) getContract(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := claims(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.Contracts == nil {
		loanContractError(w, r, loanqueries.ErrLoanContractNotReady)
		return
	}
	result, err := h.Contracts.Get(r.Context(), r.PathValue("loanId"), actor)
	if err != nil {
		loanContractError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}
