// route này hiện chưa có authentication/authorization.
package media

import (
	"net/http"

	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
)

const (
	UploadPath   = "/api/media/upload"
	DownloadPath = "/api/media/download"
)

// mux nhận mọi request đến rồi quyết định gọi hàm nào xử lý dựa trên method (GET/POST) và đường dẫn URL
type UploadRequest struct {
	Key  string `json:"key" form:"key"`
	File string `json:"file" form:"file" format:"binary"`
}

func RegisterRoutes(router *openapi.Registry, h *Handler) {
	router.HandleFunc("POST "+UploadPath, h.Upload, openapi.Operation{
		ID:                 "uploadMedia",
		Summary:            "Upload media",
		Description:        "Maximum request size is 32 MiB. This endpoint currently has no auth middleware.",
		Tags:               []string{"Media"},
		Request:            UploadRequest{},
		RequestContentType: "multipart/form-data",
		Response:           httpx.SuccessResponse[map[string]string]{},
		SuccessStatus:      http.StatusCreated,
		FailureStatuses:    []int{http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusInternalServerError},
	})
	router.HandleFunc("GET "+DownloadPath, h.Download, openapi.Operation{
		ID:                  "downloadMedia",
		Summary:             "Download media",
		Description:         "This endpoint currently has no auth middleware.",
		Tags:                []string{"Media"},
		Parameters:          []openapi.Parameter{{Name: "key", In: "query", Required: true, Type: "string"}},
		Response:            openapi.Binary{},
		ResponseContentType: "application/octet-stream",
		FailureStatuses:     []int{http.StatusBadRequest, http.StatusNotFound, http.StatusInternalServerError},
	})
}
