package http

import (
	"net/http"

	account "github.com/dont-wait/anomaly/internal/presentation/http/handler/account"
)

func NewRouter(mux *http.ServeMux, accountHandler *account.Handler) *http.ServeMux {
	account.RegisterRoutes(mux, accountHandler)

	return mux
}
