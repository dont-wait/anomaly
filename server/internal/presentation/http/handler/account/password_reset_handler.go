package account

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/dont-wait/anomaly/internal/application/account/commands"
	appotp "github.com/dont-wait/anomaly/internal/application/otp"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/domain/audit"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
)

const (
	maxPendingPasswordResets = 16
	passwordResetSendTimeout = 30 * time.Second
)

type passwordResetStartRequest struct {
	CCCDNumber string `json:"cccdNumber"`
	Email      string `json:"email"`
}

type passwordResetConfirmRequest struct {
	CCCDNumber  string `json:"cccdNumber"`
	OTP         string `json:"otp"`
	NewPassword string `json:"newPassword"`
}

// RequestPasswordReset does the lookup and email in the background: responding
// only after them would reveal through timing whether the account exists.
func (h *Handler) RequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.passwordReset == nil {
		httpx.WriteError(w, h.logger, errors.New("password reset unavailable"), func(error) int { return http.StatusServiceUnavailable })
		return
	}
	var req passwordResetStartRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, h.logger, err, func(error) int { return http.StatusBadRequest })
		return
	}
	select {
	case h.passwordResetSlots <- struct{}{}:
		requestID := r.Header.Get("X-Request-ID")
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), passwordResetSendTimeout)
		go func() {
			defer cancel()
			defer func() { <-h.passwordResetSlots }()
			h.sendPasswordReset(ctx, req, requestID)
		}()
	default:
		h.logger.Warn().Msg("password reset request dropped: too many pending")
	}
	httpx.WriteSuccess(w, http.StatusOK, "If the details match, a code has been sent", map[string]bool{"accepted": true})
}

func (h *Handler) sendPasswordReset(ctx context.Context, req passwordResetStartRequest, requestID string) {
	accountID, err := h.passwordReset.Request(ctx, req.CCCDNumber, req.Email)
	if err != nil {
		h.logger.Error().Err(err).Msg("send password reset code failed")
		return
	}
	if accountID == "" || h.profileAudit == nil {
		return
	}
	event := audit.NewEvent("PasswordResetRequested", requestID, time.Now().UTC())
	event.AccountID, event.ActorID = accountID, accountID
	if err := h.profileAudit.Record(ctx, event); err != nil {
		h.logger.Warn().Err(err).Msg("record password reset audit event failed")
	}
}

func (h *Handler) ConfirmPasswordReset(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.passwordReset == nil {
		httpx.WriteError(w, h.logger, errors.New("password reset unavailable"), func(error) int { return http.StatusServiceUnavailable })
		return
	}
	var req passwordResetConfirmRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, h.logger, err, func(error) int { return http.StatusBadRequest })
		return
	}
	accountID, err := h.passwordReset.Confirm(r.Context(), commands.ConfirmPasswordResetCommand{
		CCCDNumber:  req.CCCDNumber,
		OTP:         req.OTP,
		NewPassword: req.NewPassword,
		Source:      passwordAttemptSource(r),
	})
	if err != nil {
		switch {
		case errors.Is(err, appotp.ErrInvalid):
			h.recordProfileAudit(r, "", "PasswordResetFailed")
		case errors.Is(err, accountdomain.ErrTooManyAttempts):
			h.recordProfileAudit(r, "", "PasswordResetThrottled")
			w.Header().Set("Retry-After", strconv.Itoa(int(h.passwordRetryAfter.Seconds())))
		}
		httpx.WriteError(w, h.logger, err, accountErrorStatus, accountErrorCode)
		return
	}
	h.recordProfileAudit(r, accountID, "PasswordReset")
	httpx.WriteSuccess(w, http.StatusOK, "Password reset", map[string]bool{"changed": true})
}
