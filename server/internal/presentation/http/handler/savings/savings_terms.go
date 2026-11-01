package savings

import (
	"context"
	"log/slog"
	"net/http"

	savingsdomain "github.com/dont-wait/anomaly/internal/domain/savings"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
)

// The API response shape is unchanged; content and version now come from DB.
type TermsSnapshot = savingsdomain.TermsSnapshot
type TermsSection = savingsdomain.TermsSection

type TermsProvider interface {
	Current(context.Context) (savingsdomain.TermsSnapshot, error)
}

func (h *Handler) terms(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Terms == nil {
		replyError(w, http.StatusServiceUnavailable, "SAVINGS_TERMS_UNAVAILABLE", "savings product terms are unavailable")
		return
	}
	terms, err := h.Terms.Current(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "savings terms lookup failed", "error", err)
		replyError(w, http.StatusServiceUnavailable, "SAVINGS_TERMS_UNAVAILABLE", "savings product terms are unavailable")
		return
	}
	if err := terms.Validate(); err != nil {
		slog.ErrorContext(r.Context(), "invalid savings terms document", "error", err)
		replyError(w, http.StatusServiceUnavailable, "SAVINGS_TERMS_UNAVAILABLE", "savings product terms are unavailable")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, terms)
}
