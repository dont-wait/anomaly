package loan

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	appotp "github.com/dont-wait/anomaly/internal/application/otp"
	domain "github.com/dont-wait/anomaly/internal/domain/loan"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
	"github.com/google/uuid"
)

// The handler depends on the application port, not Redis/SMTP implementations.
type ContractOTPService = appotp.ContractOTPService
type ContractEmailReader interface {
	LoanContractEmail(context.Context, string) (string, error)
}

// Keep writes off until every old API and loan-projector instance is drained.
func offerWritesEnabled() bool { return os.Getenv("LOAN_OFFER_WRITES_ENABLED") == "true" }
func requireOfferWrites(w http.ResponseWriter) bool {
	if offerWritesEnabled() {
		return true
	}
	fail(w, 503, "loan offer rollout is not enabled")
	return false
}

func (h *Handler) issueOffer(w http.ResponseWriter, r *http.Request) {
	if !requireOfferWrites(w) {
		return
	}
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

// Offer states must be read from the authoritative event stream, even when the
// Mongo projection has not applied LoanOfferIssued yet.
func (h *Handler) offerApplication(r *http.Request, ownerOnly bool) (*domain.Application, error) {
	if h.Offer == nil || h.Offer.Repo == nil {
		return nil, errors.New("loan offer services unavailable")
	}
	id := r.PathValue("applicationId")
	if _, err := uuid.Parse(id); err != nil {
		return nil, fmt.Errorf("invalid application ID")
	}
	a, _, err := h.Offer.Repo.FindByIDWithRevision(r.Context(), id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, fmt.Errorf("loan application not found")
	}
	actor, role, ok := claims(r)
	if !ok {
		return nil, fmt.Errorf("unauthorized")
	}
	if a.CustomerId != actor {
		if ownerOnly || role != "admin" {
			return nil, fmt.Errorf("loan application not found")
		}
		if err := h.admin(r); err != nil {
			return nil, err
		}
	}
	return a, nil
}

func (h *Handler) getOffer(w http.ResponseWriter, r *http.Request) {
	a, err := h.offerApplication(r, false)
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
	if !requireOfferWrites(w) {
		return
	}
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
	a, err := h.offerApplication(r, true)
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
		if errors.Is(err, appotp.ErrTooSoon) {
			fail(w, 429, "otp request too soon")
			return
		}
		fail(w, 503, "loan contract OTP unavailable")
		return
	}
	httpx.WriteJSON(w, 200, info)
}

func (h *Handler) confirmContract(w http.ResponseWriter, r *http.Request) {
	if !requireOfferWrites(w) {
		return
	}
	a, err := h.offerApplication(r, true)
	if err != nil {
		serviceError(w, err)
		return
	}
	if a.Status != domain.ApplicationStatusApproved || a.Offer == nil || (a.Offer.Status != domain.LoanOfferAccepted && a.Offer.Status != domain.LoanOfferConfirmed) {
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
	// A committed confirmation is returned on retry without generating an
	// extra event or requiring an OTP already consumed after the first call.
	if a.Offer.Status == domain.LoanOfferConfirmed {
		httpx.WriteJSON(w, 200, a.Offer)
		return
	}
	key := contractOTPKey(a)
	_, err = h.ContractOTP.Verify(r.Context(), key, req.OTP)
	if err != nil {
		switch {
		case errors.Is(err, appotp.ErrInvalid):
			fail(w, 400, "invalid otp")
		case errors.Is(err, appotp.ErrExpired):
			fail(w, 410, "otp expired")
		case errors.Is(err, appotp.ErrExceeded):
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
