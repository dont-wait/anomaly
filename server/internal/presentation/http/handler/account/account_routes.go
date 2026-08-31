package account

import (
	"net/http"

	"github.com/dont-wait/anomaly/internal/application/account/queries"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
)

func RegisterRoutes(
	mux *http.ServeMux,
	h *Handler,
	tokenSvc queries.TokenService,
) {
	mux.HandleFunc("POST /api/auth/register", h.Register)
	mux.HandleFunc("POST /api/auth/login", h.Login)

	mux.Handle("GET /api/auth/me",
		middleware.RequireAuth(tokenSvc)(http.HandlerFunc(h.Me)))
	mux.Handle("POST /api/accounts/{id}/verify",
		middleware.RequireAuth(tokenSvc)(http.HandlerFunc(h.Verify)))

	mux.HandleFunc("GET /api/accounts", h.GetAll)
	mux.HandleFunc("GET /api/accounts/by-email/{email}", h.GetByEmail)
	mux.HandleFunc("GET /api/accounts/{id}", h.GetByID)
}
