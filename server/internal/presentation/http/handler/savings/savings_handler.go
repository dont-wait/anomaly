package savings

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	savingscommands "github.com/dont-wait/anomaly/internal/application/savings/commands"
	savingsqueries "github.com/dont-wait/anomaly/internal/application/savings/queries"
	savingsdomain "github.com/dont-wait/anomaly/internal/domain/savings"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
)

type Handler struct {
	Service  *savingsqueries.Service
	Commands *savingscommands.Service
	Read     *savingsqueries.SavingsReadService
	Terms    TermsProvider
}

type apiError struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func replyError(w http.ResponseWriter, status int, code, message string) {
	httpx.WriteJSON(w, status, apiError{Error: message, Code: code})
}

func (h *Handler) packages(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Service == nil {
		replyError(w, http.StatusServiceUnavailable, "SAVINGS_UNAVAILABLE", "savings service unavailable")
		return
	}
	items, err := h.Service.ListPackages(r.Context())
	if err != nil {
		if errors.Is(err, savingsqueries.ErrPackageUnavailable) {
			replyError(w, http.StatusServiceUnavailable, "SAVINGS_POLICY_UNAVAILABLE", "savings catalog is not consistent with approved policy")
			return
		}
		slog.ErrorContext(r.Context(), "savings dependency failure", "operation", "list_savings_packages", "error", err)
		replyError(w, http.StatusServiceUnavailable, "SAVINGS_UNAVAILABLE", "savings catalog unavailable")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, struct {
		Items []savingsqueries.Package `json:"items"`
	}{Items: items})
}

func (h *Handler) quote(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Service == nil {
		replyError(w, http.StatusServiceUnavailable, "SAVINGS_UNAVAILABLE", "savings service unavailable")
		return
	}
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil || strings.TrimSpace(claims.UserID) == "" {
		replyError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	var req savingsqueries.CreateQuoteInput
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		replyError(w, http.StatusBadRequest, "INVALID_QUOTE_REQUEST", "invalid quote JSON")
		return
	}
	result, err := h.Service.CreateQuote(r.Context(), claims.UserID, req)
	if err != nil {
		switch {
		case errors.Is(err, savingsqueries.ErrInvalidAmount), errors.Is(err, savingsdomain.ErrAmountOverflow):
			replyError(w, http.StatusBadRequest, "INVALID_AMOUNT", "invalid savings amount")
		case errors.Is(err, savingsqueries.ErrMinimumDeposit), errors.Is(err, savingsdomain.ErrInvalidDepositAmount):
			replyError(w, http.StatusUnprocessableEntity, "BELOW_MINIMUM_DEPOSIT", "minimum deposit is 1000000 VND")
		case errors.Is(err, savingsdomain.ErrUnsupportedTerm):
			replyError(w, http.StatusBadRequest, "UNSUPPORTED_TERM", "unsupported savings term")
		case errors.Is(err, savingsqueries.ErrPackageUnavailable):
			replyError(w, http.StatusNotFound, "SAVINGS_PACKAGE_NOT_FOUND", "savings package is not available")
		case errors.Is(err, savingsqueries.ErrInvalidSource):
			replyError(w, http.StatusUnprocessableEntity, "INVALID_SOURCE_ACCOUNT", "payment account unavailable")
		case errors.Is(err, savingsqueries.ErrInsufficientFunds):
			replyError(w, http.StatusBadRequest, "INSUFFICIENT_FUNDS", "insufficient funds")
		default:
			slog.ErrorContext(r.Context(), "savings dependency failure", "operation", "create_savings_quote", "error", err)
			replyError(w, http.StatusServiceUnavailable, "SAVINGS_QUOTE_UNAVAILABLE", "savings quote unavailable")
		}
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}
