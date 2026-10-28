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

func (h *Handler) actor(w http.ResponseWriter, r *http.Request) (string, bool) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil || strings.TrimSpace(claims.UserID) == "" {
		replyError(w, 401, "UNAUTHORIZED", "authentication required")
		return "", false
	}
	if h == nil || h.Commands == nil {
		replyError(w, 503, "SAVINGS_UNAVAILABLE", "savings commands unavailable")
		return "", false
	}
	return claims.UserID, true
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, operation string, err error) {
	switch {
	case errors.Is(err, savingscommands.ErrConfirmationRequired):
		replyError(w, 400, "CONFIRMATION_REQUIRED", "confirmation required")

	case errors.Is(err, savingscommands.ErrIdempotencyConflict):
		replyError(w, 409, "IDEMPOTENCY_CONFLICT", "idempotency key used for another request")

	case errors.Is(err, savingscommands.ErrUnsupportedWithdrawalType):
		replyError(w, 400, "UNSUPPORTED_WITHDRAWAL_TYPE", "only full withdrawals supported")

	case errors.Is(err, savingscommands.ErrSavingsNotSettled):
		replyError(w, 409, "SAVINGS_NOT_SETTLED", "savings contract not settled")

	case errors.Is(err, savingscommands.ErrSavingsSettlementUnconfirmed):
		slog.WarnContext(r.Context(), "savings withdrawal confirmation unavailable", "operation", operation, "error", err)
		w.Header().Set("Retry-After", "2")
		replyError(w, 503, "SETTLEMENT_UNAVAILABLE", "withdrawal outcome is unknown; check settlement receipt or retry with the same idempotency key")

	case errors.Is(err, savingscommands.ErrSavingsWritesDisabled):
		slog.WarnContext(r.Context(), "savings writes disabled", "operation", operation)
		replyError(w, 503, "SAVINGS_WRITES_DISABLED", "savings operations not yet enabled")

	case errors.Is(err, savingscommands.ErrInvalidKey):
		replyError(w, 400, "INVALID_IDEMPOTENCY_KEY", "valid UUID v4 Idempotency-Key required")

	case errors.Is(err, savingsqueries.ErrQuoteNotFound):
		replyError(w, 404, "QUOTE_NOT_FOUND", "quote not found")

	case errors.Is(err, savingsqueries.ErrQuoteExpired),
		errors.Is(err, savingscommands.ErrWithdrawalQuoteExpired):
		replyError(w, 410, "QUOTE_EXPIRED", "quote expired")

	case errors.Is(err, savingscommands.ErrSavingsNotFound):
		replyError(w, 404, "SAVINGS_NOT_FOUND", "savings contract not found")

	case errors.Is(err, savingsdomain.ErrAlreadySettled):
		replyError(w, 409, "SAVINGS_ALREADY_SETTLED", "savings contract already settled")

	case errors.Is(err, savingsdomain.ErrNotMatured):
		replyError(w, 422, "SAVINGS_NOT_MATURED", "savings contract not mature")

	case errors.Is(err, savingsdomain.ErrContractLocked):
		replyError(w, 423, "SAVINGS_LOCKED", "savings contract locked")

	case errors.Is(err, savingscommands.ErrDestinationUnavailable),
		errors.Is(err, savingscommands.ErrSavingsSourceUnavailable):
		replyError(w, 422, "DESTINATION_ACCOUNT_UNAVAILABLE", "payment account unavailable")

	case errors.Is(err, savingscommands.ErrSavingsInsufficientFunds),
		errors.Is(err, savingsqueries.ErrInsufficientFunds):
		replyError(w, 400, "INSUFFICIENT_FUNDS", "insufficient funds")

	case errors.Is(err, savingscommands.ErrInvalidWithdrawalQuote),
		errors.Is(err, savingsqueries.ErrQuoteInvalid):
		replyError(w, 400, "INVALID_QUOTE", "invalid quote request")

	case errors.Is(err, savingsqueries.ErrInvalidSource):
		replyError(w, 422, "INVALID_SOURCE_ACCOUNT", "source account unavailable")

	default:
		slog.ErrorContext(
			r.Context(),
			"savings dependency failure",
			"operation", operation,
			"error", err,
		)
		replyError(w, 503, "SAVINGS_COMMAND_UNAVAILABLE", "unable to confirm banking command safely")
	}
}

func (h *Handler) openContract(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	var req savingscommands.OpenInput
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		replyError(w, 400, "INVALID_REQUEST", "invalid JSON")
		return
	}
	result, err := h.Commands.Open(r.Context(), actor, r.Header.Get("Idempotency-Key"), req)
	if err != nil {
		h.fail(w, r, "open_savings_contract", err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, result)
}

func (h *Handler) withdrawalQuote(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	var req struct {
		WithdrawalType string `json:"withdrawalType"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		replyError(w, 400, "INVALID_REQUEST", "invalid JSON")
		return
	}
	result, err := h.Commands.WithdrawalPreview(r.Context(), actor, r.PathValue("id"), req.WithdrawalType)
	if err != nil {
		h.fail(w, r, "create_withdrawal_quote", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) withdraw(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	var req savingscommands.WithdrawInput
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		replyError(w, 400, "INVALID_REQUEST", "invalid JSON")
		return
	}
	result, err := h.Commands.Withdraw(r.Context(), actor, r.PathValue("id"), r.Header.Get("Idempotency-Key"), req)
	if err != nil {
		h.fail(w, r, "withdraw_savings", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) settlement(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	result, err := h.Commands.Receipt(r.Context(), actor, r.PathValue("id"))
	if err != nil {
		h.fail(w, r, "get_savings_settlement", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}
