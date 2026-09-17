package http

import (
	"net/http"

	"github.com/dont-wait/anomaly/internal/application/account/queries"
	account "github.com/dont-wait/anomaly/internal/presentation/http/handler/account"
	media "github.com/dont-wait/anomaly/internal/presentation/http/handler/media"
	otp "github.com/dont-wait/anomaly/internal/presentation/http/handler/otp"
)

// NewRouter đăng ký routes cho account handler, media handler, và otp handler.
// Token service được truyền qua queries.TokenService (port từ application)
// chứ không phải concrete *auth.TokenService — tránh presentation phụ
// thuộc trực tiếp vào infrastructure, đúng dependency rule của Clean
// Architecture.
func NewRouter(mux *http.ServeMux, accountHandler *account.Handler, mediaHandler *media.Handler, otpHandler *otp.Handler, tokenSvc queries.TokenService) *http.ServeMux {
	account.RegisterRoutes(mux, accountHandler, tokenSvc)
	media.RegisterRoutes(mux, mediaHandler)
	otp.RegisterRoutes(mux, otpHandler)

	return mux
}
