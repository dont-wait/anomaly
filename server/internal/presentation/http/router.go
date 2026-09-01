package http

import (
	"net/http"

	"github.com/dont-wait/anomaly/internal/infrastructure/auth"
	account "github.com/dont-wait/anomaly/internal/presentation/http/handler/account"
	media "github.com/dont-wait/anomaly/internal/presentation/http/handler/media"
)

func NewRouter(mux *http.ServeMux, accountHandler *account.Handler, mediaHandler *media.Handler, tokenSvc *auth.TokenService) *http.ServeMux {
	account.RegisterRoutes(mux, accountHandler, tokenSvc)
	media.RegisterRoutes(mux, mediaHandler)

	return mux
}
