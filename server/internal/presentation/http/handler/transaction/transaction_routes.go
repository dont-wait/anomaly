package transaction

import (
	"net/http"

	"github.com/dont-wait/anomaly/internal/application/account/queries"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler, tokenSvc queries.TokenService) {
	mux.Handle("POST /api/transfer",
		middleware.RequireAuth(tokenSvc)(http.HandlerFunc(h.Transfer)))
	mux.Handle("GET /api/transactions",
		middleware.RequireAuth(tokenSvc)(http.HandlerFunc(h.GetFeed)))
}
