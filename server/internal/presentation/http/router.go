package http

import (
	"net/http"

	"github.com/dont-wait/anomaly/internal/application/account/queries"

	account "github.com/dont-wait/anomaly/internal/presentation/http/handler/account"
	loan "github.com/dont-wait/anomaly/internal/presentation/http/handler/loan"
	media "github.com/dont-wait/anomaly/internal/presentation/http/handler/media"
	otp "github.com/dont-wait/anomaly/internal/presentation/http/handler/otp"
	savings "github.com/dont-wait/anomaly/internal/presentation/http/handler/savings"
	transaction "github.com/dont-wait/anomaly/internal/presentation/http/handler/transaction"

	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
)

// NewRouter đăng ký routes cho account handler, media handler, và otp handler.
// Token service được truyền qua queries.TokenService (port từ application)
// chứ không phải concrete *auth.TokenService — tránh presentation phụ
// thuộc trực tiếp vào infrastructure, đúng dependency rule của Clean
// Architecture.
func NewRouter(mux *http.ServeMux, accountHandler *account.Handler, mediaHandler *media.Handler, otpHandler *otp.Handler, transactionHandler *transaction.Handler, tokenSvc queries.TokenService, swaggerEnabled bool, loanHandlers ...*loan.Handler) *http.ServeMux {
	return NewRouterWithSavings(mux, accountHandler, mediaHandler, otpHandler, transactionHandler, tokenSvc, swaggerEnabled, nil, loanHandlers...)
}

// NewRouterWithSavings adds the savings GĐ2 API without changing existing NewRouter callers.
func NewRouterWithSavings(mux *http.ServeMux, accountHandler *account.Handler, mediaHandler *media.Handler, otpHandler *otp.Handler, transactionHandler *transaction.Handler, tokenSvc queries.TokenService, swaggerEnabled bool, savingsHandler *savings.Handler, loanHandlers ...*loan.Handler) *http.ServeMux {
	router := openapi.NewRegistry(mux, swaggerEnabled)
	account.RegisterRoutes(router, accountHandler, tokenSvc)
	media.RegisterRoutes(router, mediaHandler, tokenSvc)
	otp.RegisterRoutes(router, otpHandler)
	transaction.RegisterRoutes(router, transactionHandler, tokenSvc)
	if len(loanHandlers) > 0 &&
		loanHandlers[0] != nil {

		loan.RegisterRoutes(
			router,
			loanHandlers[0],
			tokenSvc,
		)
	}
	router.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}, openapi.Operation{
		ID:                  "healthCheck",
		Summary:             "Health check",
		Tags:                []string{"Health"},
		Response:            "OK",
		ResponseContentType: "text/plain",
	})
	if savingsHandler != nil {
		savings.RegisterRoutes(router, savingsHandler, tokenSvc)
	}
	router.RegisterDocs()

	return mux
}
