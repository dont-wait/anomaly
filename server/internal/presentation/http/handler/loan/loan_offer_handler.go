package loan

import (
	"context"
	"errors"
	"net/http"

	domain "github.com/dont-wait/anomaly/internal/domain/loan"
	"github.com/dont-wait/anomaly/internal/infrastructure/otp"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
)

type ContractOTPService interface {
	Issue(context.Context, string, string) (otp.TransferOTPInfo, error)
	Verify(context.Context, string, string) (int, error)
	Consume(context.Context, string) error
}
type ContractEmailReader interface {
	LoanContractEmail(context.Context, string) (string, error)
}

func (h *Handler) issueOffer(w http.ResponseWriter, r *http.Request) {
	if err := h.admin(r); err != nil {
		serviceError(w, err)
		return
	}
	a, err := h.application(r, false)
	if err != nil {
		serviceError(w, err)
		return
	}
	if h.Offer == nil {
		fail(w, 503, "loan offer service unavailable")
		return
	}
	actor, _, _ := claims(r)
	result, err := h.Offer.Issue(r.Context(), a.Id, actor)
	if err != nil {
		serviceError(w, err)
		return
	}
	httpx.WriteJSON(w, 200, result.Offer)
}

func (h *Handler) getOffer(w http.ResponseWriter, r *http.Request) {
	a, err := h.application(r, false)
	if err != nil {
		serviceError(w, err)
		return
	}
	if a.Offer == nil {
		fail(w, 404, "loan offer not found")
		return
	}
	httpx.WriteJSON(w, 200, a.Offer)
}

func (h *Handler) acceptOffer(w http.ResponseWriter, r *http.Request) {
	h.transitionOffer(w, r, "accept")
}

func (h *Handler) rejectOffer(w http.ResponseWriter, r *http.Request) {
	h.transitionOffer(w, r, "reject")
}

func (h *Handler) transitionOffer(w http.ResponseWriter, r *http.Request, kind string) {
	a, err := h.application(r, true)
	if err != nil {
		serviceError(w, err)
		return
	}
	if h.Offer == nil {
		fail(w, 503, "loan offer service unavailable")
		return
	}
	owner, _, _ := claims(r)
	var result *domain.Application
	if kind == "accept" {
		result, err = h.Offer.Accept(r.Context(), a.Id, owner)
	} else {
		result, err = h.Offer.Reject(r.Context(), a.Id, owner)
	}
	if err != nil {
		serviceError(w, err)
		return
	}
	httpx.WriteJSON(w, 200, result.Offer)
}

func contractOTPKey(a *domain.Application) string {
	return a.Id + ":" + a.Offer.IssuedAt.UTC().Format("20060102T150405.000000000Z")
}

func (h *Handler) issueContractOTP(w http.ResponseWriter, r *http.Request) {
	a, err := h.application(r, true)
	if err != nil {
		serviceError(w, err)
		return
	}
	if a.Status != domain.ApplicationStatusApproved || a.Offer == nil || a.Offer.Status != domain.LoanOfferAccepted {
		serviceError(w, domain.ErrLoanOfferNotAccepted)
		return
	}
	if h.ContractOTP == nil || h.ContractEmail == nil {
		fail(w, 503, "loan contract OTP service unavailable")
		return
	}
	email, err := h.ContractEmail.LoanContractEmail(r.Context(), a.CustomerId)
	if err != nil {
		serviceError(w, err)
		return
	}
	info, err := h.ContractOTP.Issue(r.Context(), contractOTPKey(a), email)
	if err != nil {
		if errors.Is(err, otp.ErrTransferOTPTooSoon) {
			fail(w, 429, "otp request too soon")
			return
		}
		fail(w, 503, "loan contract OTP unavailable")
		return
	}
	httpx.WriteJSON(w, 200, info)
}

func (h *Handler) confirmContract(w http.ResponseWriter, r *http.Request) {
	a, err := h.application(r, true)
	if err != nil {
		serviceError(w, err)
		return
	}
	if a.Status != domain.ApplicationStatusApproved || a.Offer == nil || a.Offer.Status != domain.LoanOfferAccepted {
		serviceError(w, domain.ErrLoanOfferNotAccepted)
		return
	}
	if h.ContractOTP == nil || h.Offer == nil {
		fail(w, 503, "loan contract confirmation unavailable")
		return
	}
	var req struct {
		OTP         string `json:"otp"`
		AcceptTerms bool   `json:"acceptTerms"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		fail(w, 400, "invalid contract confirmation JSON")
		return
	}
	if !req.AcceptTerms {
		fail(w, 422, "loan contract terms must be accepted")
		return
	}
	key := contractOTPKey(a)
	_, err = h.ContractOTP.Verify(r.Context(), key, req.OTP)
	if err != nil {
		switch {
		case errors.Is(err, otp.ErrTransferOTPInvalid):
			fail(w, 400, "invalid otp")
		case errors.Is(err, otp.ErrTransferOTPExpired):
			fail(w, 410, "otp expired")
		case errors.Is(err, otp.ErrTransferOTPExceeded):
			fail(w, 423, "otp attempts exceeded")
		default:
			fail(w, 503, "loan contract OTP service unavailable")
		}
		return
	}
	owner, _, _ := claims(r)
	result, err := h.Offer.Confirm(r.Context(), a.Id, owner)
	if err != nil {
		serviceError(w, err)
		return
	}
	// Keep verified authorization for safe retry if Redis cleanup fails.
	_ = h.ContractOTP.Consume(r.Context(), key)
	httpx.WriteJSON(w, 200, result.Offer)
}
