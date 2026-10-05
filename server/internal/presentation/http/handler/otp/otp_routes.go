package otp

import (
	"net/http"

	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
)

func RegisterRoutes(router *openapi.Registry, h *Handler) {
	router.HandleFunc("POST /api/auth/otp/request", h.RequestOTP, openapi.Operation{
		ID:              "requestOTP",
		Summary:         "Request an OTP by email",
		Tags:            []string{"Auth"},
		Request:         requestOTPRequest{},
		Response:        httpx.SuccessResponse[map[string]any]{},
		FailureStatuses: []int{http.StatusBadRequest, http.StatusInternalServerError},
	})
	router.HandleFunc("POST /api/auth/otp/verify", h.VerifyOTP, openapi.Operation{
		ID:              "verifyOTP",
		Summary:         "Verify an OTP",
		Tags:            []string{"Auth"},
		Request:         verifyOTPRequest{},
		Response:        httpx.SuccessResponse[map[string]bool]{},
		FailureStatuses: []int{http.StatusBadRequest, http.StatusGone, http.StatusInternalServerError},
	})
}
