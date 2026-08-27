// route này hiện chưa có authentication/authorization.
package media

import "net/http"

const (
	UploadPath   = "/api/media/upload"
	DownloadPath = "/api/media/download"
)

// mux nhận mọi request đến rồi quyết định gọi hàm nào xử lý dựa trên method (GET/POST) và đường dẫn URL
func RegisterRoutes(
	mux *http.ServeMux,
	h *Handler,
) {
	mux.HandleFunc("POST "+UploadPath, h.Upload) // hễ có request POST tới đúng URL, gọi hàm h.Upload xử lý
	mux.HandleFunc("GET "+DownloadPath, h.Download)
}
