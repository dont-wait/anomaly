package transaction

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/rs/zerolog"

	"github.com/dont-wait/anomaly/internal/application/transaction/commands"
	"github.com/dont-wait/anomaly/internal/application/transaction/queries"
	txdomain "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
)

type Handler struct {
	logger   zerolog.Logger
	transfer *commands.TransferCommandHandler
	feed     *queries.ListAccountFeedQueryHandler
}

func NewHandler(
	logger zerolog.Logger,
	transfer *commands.TransferCommandHandler,
	feed *queries.ListAccountFeedQueryHandler,
) *Handler {
	return &Handler{logger: logger, transfer: transfer, feed: feed}
}

func transactionErrorStatus(err error) int {
	switch {
	case errors.Is(err, txdomain.ErrSourceAccountNotFound), errors.Is(err, txdomain.ErrDestAccountNotFound):
		return http.StatusNotFound
	case errors.Is(err, txdomain.ErrInvalidAmount), errors.Is(err, txdomain.ErrSameAccount), errors.Is(err, txdomain.ErrIdempotencyKeyRequired):
		return http.StatusBadRequest
	case errors.Is(err, txdomain.ErrInsufficientFunds):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

type transferRequest struct {
	DestAccountNo  string `json:"destAccountNo"`
	Amount         int64  `json:"amount"`
	IdempotencyKey string `json:"idempotencyKey"`
}

// Transfer — POST /api/accounts/{id}/transfer, chỉ chủ tài khoản mới gọi được.
func (h *Handler) Transfer(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httpx.WriteError(w, h.logger, errors.New("missing claims"), func(error) int { return http.StatusUnauthorized })
		return
	}
	id := claims.UserID

	var req transferRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, h.logger, err, func(error) int { return http.StatusBadRequest })
		return
	}

	tx, err := h.transfer.Handle(r.Context(), commands.TransferCommand{
		SourceAccountId: id,
		DestAccountNo:   req.DestAccountNo,
		Amount:          req.Amount,
		IdempotencyKey:  req.IdempotencyKey,
	})
	if err != nil {
		httpx.WriteError(w, h.logger, err, transactionErrorStatus)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, toTransferResponse(tx))
}

// GetFeed — GET /api/accounts/{id}/transactions?limit=20, chỉ chủ tài khoản
// mới xem được lịch sử giao dịch của chính mình.
func (h *Handler) GetFeed(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httpx.WriteError(w, h.logger, errors.New("missing claims"), func(error) int { return http.StatusUnauthorized })
		return
	}
	id := claims.UserID

	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}

	entries, err := h.feed.Handle(r.Context(), queries.ListAccountFeedQuery{AccountId: id, Limit: limit})
	if err != nil {
		httpx.WriteError(w, h.logger, err, transactionErrorStatus)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, toFeedEntryResponseList(entries))
}
