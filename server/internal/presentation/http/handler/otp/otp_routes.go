package otp

import (
	"net/http"
)

func RegisterRoutes(mux *http.ServeMux, h *Handler) {
	mux.HandleFunc("POST /api/auth/otp/request", h.RequestOTP)
	mux.HandleFunc("POST /api/auth/otp/verify", h.VerifyOTP)
}
