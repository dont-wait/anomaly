package media

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/rs/zerolog"

	"github.com/dont-wait/anomaly/internal/application/account/queries"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/infrastructure/rustfs"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
)

// khai báo
type Handler struct {
	logger   zerolog.Logger          // ghi log lỗi
	repo     *rustfs.MediaRepository // công cụ Upload/Download
	accounts queries.AccountQueryRepository
}

// nhận log và repo và tra về handler
func NewHandler(logger zerolog.Logger, repo *rustfs.MediaRepository, accounts ...queries.AccountQueryRepository) *Handler {
	handler := &Handler{logger: logger, repo: repo}
	if len(accounts) > 0 {
		handler.accounts = accounts[0]
	}
	return handler
}

// Download lấy từ URL vì đây là request GET (không có body chứa form data như POST)
func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		httpx.WriteError(w, h.logger, errors.New("missing key"), func(error) int {
			return http.StatusBadRequest
		}, func(error) httpx.ErrorCode { return httpx.ErrorCodeMissingKey })
		return
	}
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httpx.WriteError(w, h.logger, middleware.ErrInvalidToken, func(error) int { return http.StatusUnauthorized })
		return
	}
	parts := strings.Split(key, "/")
	if len(parts) != 4 || parts[0] != "kyc" || parts[1] != claims.UserID ||
		(parts[2] != "id-card-front" && parts[2] != "id-card-back" && parts[2] != "live-video") || parts[3] == "" {
		httpx.WriteError(w, h.logger, errors.New("media access forbidden"), func(error) int { return http.StatusForbidden }, func(error) httpx.ErrorCode {
			return httpx.ErrorCodeForbidden
		})
		return
	}
	if h.accounts == nil {
		httpx.WriteError(w, h.logger, errors.New("account repository unavailable"), func(error) int { return http.StatusInternalServerError })
		return
	}
	account, err := h.accounts.FindByID(r.Context(), claims.UserID)
	if err != nil {
		httpx.WriteError(w, h.logger, err, func(error) int { return http.StatusInternalServerError })
		return
	}
	if account == nil || !account.IsVerified() || account.Customer == nil {
		httpx.WriteError(w, h.logger, accountdomain.ErrAccountNotFound, func(error) int { return http.StatusForbidden }, func(error) httpx.ErrorCode { return httpx.ErrorCodeForbidden })
		return
	}
	session := account.VerifiedKYCSession()
	if session == nil || (key != session.Media.IdentityFront.StorageKey && key != session.Media.IdentityBack.StorageKey && key != session.Media.LivenessVideo.StorageKey) {
		httpx.WriteError(w, h.logger, errors.New("media access forbidden"), func(error) int { return http.StatusForbidden }, func(error) httpx.ErrorCode { return httpx.ErrorCodeForbidden })
		return
	}

	result, err := h.repo.Download(r.Context(), key)
	if err != nil {
		httpx.WriteError(w, h.logger, err, func(err error) int {
			if errors.Is(err, rustfs.ErrObjectNotFound) {
				return http.StatusNotFound
			}
			return http.StatusInternalServerError
		})
		return
	}
	defer func() {
		if err := result.Body.Close(); err != nil {
			h.logger.Warn().Err(err).Msg("close download body failed")
		}
	}()

	w.Header().Set("Content-Type", result.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": path.Base(key)}))
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, result.Body); err != nil {
		h.logger.Error().Err(err).Msg("stream download response failed")
	}
}
