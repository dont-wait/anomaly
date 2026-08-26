package media

import "net/http"

// mux nhận mọi request đến rồi quyết định gọi hàm nào xử lý dựa trên method (GET/POST) và đường dẫn URL
func RegisterRoutes(
	mux *http.ServeMux,
	h *Handler,
) {
	mux.HandleFunc("POST /api/media/upload", h.Upload) // hễ có request POST tới đúng URL, gọi hàm h.Upload xử lý
	mux.HandleFunc("GET /api/media/download", h.Download)
}
