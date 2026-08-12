package http

import (
	"net/http"

	account "github.com/dont-wait/anomaly/internal/presentation/http/handler/account"
)

func NewRouter(
	accountHandler *account.Handler,
) http.Handler {
	mux := http.NewServeMux()

	account.RegisterRoutes(mux, accountHandler)

	return mux
}
