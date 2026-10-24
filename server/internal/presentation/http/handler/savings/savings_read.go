package savings

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	savingsqueries "github.com/dont-wait/anomaly/internal/application/savings/queries"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
)

func (h *Handler) readActor(w http.ResponseWriter, r *http.Request) (string, bool) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil || strings.TrimSpace(claims.UserID) == "" {
		replyError(w, 401, "UNAUTHORIZED", "authentication required")
		return "", false
	}
	if h == nil || h.Read == nil {
		replyError(w, 503, "SAVINGS_READ_UNAVAILABLE", "savings read service unavailable")
		return "", false
	}
	return claims.UserID, true
}

func (h *Handler) readError(w http.ResponseWriter, r *http.Request, op string, err error) {
	switch {
	case errors.Is(err, savingsqueries.ErrInvalidRead):
		replyError(w, 400, "INVALID_SAVINGS_QUERY", "invalid savings query")
	case errors.Is(err, savingsqueries.ErrContractNotFound):
		replyError(w, 404, "SAVINGS_NOT_FOUND", "savings contract not found")
	case errors.Is(err, savingsqueries.ErrReadChanged):
		replyError(w, 409, "SAVINGS_READ_CHANGED", "savings data changed, restart pagination")
	case errors.Is(err, savingsqueries.ErrReadNotReady):
		slog.ErrorContext(r.Context(), "savings read projection not ready", "operation", op, "error", err)
		replyError(w, 503, "SAVINGS_READ_NOT_READY", "savings projection not ready")
	default:
		slog.ErrorContext(r.Context(), "savings read failed", "operation", op, "error", err)
		replyError(w, 503, "SAVINGS_READ_UNAVAILABLE", "savings read unavailable")
	}
}

func readLimit(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 20, nil
	}
	return strconv.Atoi(raw)
}

func (h *Handler) listContracts(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.readActor(w, r)
	if !ok {
		return
	}
	n, err := readLimit(r)
	if err != nil {
		h.readError(w, r, "list_savings", savingsqueries.ErrInvalidRead)
		return
	}
	page, err := h.Read.List(r.Context(), actor, r.URL.Query().Get("status"), r.URL.Query().Get("cursor"), n)
	if err != nil {
		h.readError(w, r, "list_savings", err)
		return
	}
	httpx.WriteJSON(w, 200, page)
}

func (h *Handler) contractDetail(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.readActor(w, r)
	if !ok {
		return
	}
	record, err := h.Read.Detail(r.Context(), actor, r.PathValue("id"))
	if err != nil {
		h.readError(w, r, "savings_detail", err)
		return
	}
	httpx.WriteJSON(w, 200, record)
}

func (h *Handler) contractHistory(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.readActor(w, r)
	if !ok {
		return
	}
	rows, err := h.Read.History(r.Context(), actor, r.PathValue("id"))
	if err != nil {
		h.readError(w, r, "savings_history", err)
		return
	}
	httpx.WriteJSON(w, 200, rows)
}

func (h *Handler) maturityNotifications(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.readActor(w, r)
	if !ok {
		return
	}
	n, err := readLimit(r)
	if err != nil {
		h.readError(w, r, "savings_notifications", savingsqueries.ErrInvalidRead)
		return
	}
	rows, err := h.Read.Notifications(r.Context(), actor, n)
	if err != nil {
		h.readError(w, r, "savings_notifications", err)
		return
	}
	httpx.WriteJSON(w, 200, rows)
}

func (h *Handler) RegisterReadRoutes(auth func(string, http.HandlerFunc, openapi.Operation)) {
	auth("GET /api/savings/contracts", h.listContracts, openapi.Operation{
		ID: "listSavingsContracts", Summary: "List customer savings with runtime maturity status", Response: savingsqueries.SavingsPage{}, FailureStatuses: []int{400, 401, 409, 503},
	})
	auth("GET /api/savings/contracts/{id}", h.contractDetail, openapi.Operation{
		ID: "getSavingsContract", Summary: "Read customer-owned savings contract", Response: savingsqueries.SavingsContract{}, FailureStatuses: []int{400, 401, 404, 409, 503},
	})
	auth("GET /api/savings/contracts/{id}/history", h.contractHistory, openapi.Operation{
		ID: "listSavingsHistory", Summary: "Read savings debit and settlement ledger", Response: savingsqueries.SavingsHistory{}, FailureStatuses: []int{400, 401, 404, 503},
	})
	auth("GET /api/savings/notifications", h.maturityNotifications, openapi.Operation{
		ID: "listSavingsNotifications", Summary: "Read one-time in-app maturity notifications", Response: savingsqueries.SavingsNotices{}, FailureStatuses: []int{400, 401, 503},
	})
}
