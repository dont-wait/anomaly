package http

import (
	"net/http"

	"github.com/dont-wait/anomaly/internal/infrastructure/auth"
	account "github.com/dont-wait/anomaly/internal/presentation/http/handler/account"
)

func NewRouter(mux *http.ServeMux, accountHandler *account.Handler, tokenSvc *auth.TokenService) *http.ServeMux {
	account.RegisterRoutes(mux, accountHandler, tokenSvc)

	return mux
}
