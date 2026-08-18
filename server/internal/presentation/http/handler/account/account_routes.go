package account

import "net/http"

func RegisterRoutes(
	mux *http.ServeMux,
	h *Handler,
) {
	mux.HandleFunc("POST /api/accounts", h.Create)
	mux.HandleFunc("GET /api/accounts", h.GetAll)
	mux.HandleFunc("GET /api/accounts/by-email/{email}", h.GetByEmail)
	mux.HandleFunc("GET /api/accounts/{id}", h.GetByID)
}
