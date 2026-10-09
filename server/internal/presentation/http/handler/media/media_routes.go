package media

import (
	"net/http"

	"github.com/dont-wait/anomaly/internal/application/account/queries"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
)

const (
	DownloadPath = "/api/media/download"
)

// mux nhận mọi request đến rồi quyết định gọi hàm nào xử lý dựa trên method (GET/POST) và đường dẫn URL
func RegisterRoutes(router *openapi.Registry, h *Handler, tokenSvc queries.TokenService) {
	router.Handle("GET "+DownloadPath, middleware.RequireAuth(tokenSvc)(http.HandlerFunc(h.Download)), openapi.Operation{
		ID:                  "downloadMedia",
		Summary:             "Download media",
		Description:         "Requires an access token. Only the current owner can download a recognized account-scoped KYC object.",
		Tags:                []string{"Media"},
		Parameters:          []openapi.Parameter{{Name: "key", In: "query", Required: true, Type: "string"}},
		Response:            openapi.Binary{},
		ResponseContentType: "application/octet-stream",
		FailureStatuses:     []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError},
		Auth:                true,
	})
}
