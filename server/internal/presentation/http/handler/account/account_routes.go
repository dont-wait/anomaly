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

	mux.Handle("GET /api/accounts",
		middleware.RequireAuth(tokenSvc)(http.HandlerFunc(h.GetAll)))
	mux.Handle("GET /api/accounts/by-email/{email}",
		middleware.RequireAuth(tokenSvc)(http.HandlerFunc(h.GetByEmail)))
	mux.Handle("GET /api/accounts/{id}",
		middleware.RequireAuth(tokenSvc)(http.HandlerFunc(h.GetByID)))
}
