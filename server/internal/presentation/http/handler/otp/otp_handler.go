package otp

import (
	"errors"
	"net/http"

	"github.com/rs/zerolog"

	appotp "github.com/dont-wait/anomaly/internal/application/otp"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	otpdomain "github.com/dont-wait/anomaly/internal/domain/otp"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
)

type Handler struct {
	logger  zerolog.Logger
	request *appotp.RequestOTPCommandHandler
	verify  *appotp.VerifyOTPHandler
}

func NewHandler(
	logger zerolog.Logger,
	request *appotp.RequestOTPCommandHandler,
	verify *appotp.VerifyOTPHandler,
) *Handler {
	return &Handler{logger: logger, request: request, verify: verify}
}

func otpErrorStatus(err error) int {
	switch {
	case errors.Is(err, accountdomain.ErrInvalidEmail),
		errors.Is(err, otpdomain.ErrOTPInvalid):
		return http.StatusBadRequest
	case errors.Is(err, otpdomain.ErrOTPExpired):
		return http.StatusGone
	default:
		return http.StatusInternalServerError
	}
}

type requestOTPRequest struct {
	Email string `json:"email"`
}

type requestOTPResponse struct {
	Message string `json:"message"`
}

func (h *Handler) RequestOTP(w http.ResponseWriter, r *http.Request) {
	var req requestOTPRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, h.logger, err, func(err error) int {
			if errors.Is(err, httpx.ErrBodyTooLarge) {
				return http.StatusRequestEntityTooLarge
			}
			return http.StatusBadRequest
		})
		return
	}

	if err := h.request.Handle(r.Context(), appotp.RequestOTPCommand{Email: req.Email}); err != nil {
		httpx.WriteError(w, h.logger, err, otpErrorStatus)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, requestOTPResponse{Message: "otp sent"})
}

type verifyOTPRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

type verifyOTPResponse struct {
	Verified bool `json:"verified"`
}

func (h *Handler) VerifyOTP(w http.ResponseWriter, r *http.Request) {
	var req verifyOTPRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, h.logger, err, func(err error) int {
			if errors.Is(err, httpx.ErrBodyTooLarge) {
				return http.StatusRequestEntityTooLarge
			}
			return http.StatusBadRequest
		})
		return
	}

	if err := h.verify.Handle(r.Context(), appotp.VerifyOTPCommand{Email: req.Email, Code: req.Code}); err != nil {
		httpx.WriteError(w, h.logger, err, otpErrorStatus)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, verifyOTPResponse{Verified: true})
}
