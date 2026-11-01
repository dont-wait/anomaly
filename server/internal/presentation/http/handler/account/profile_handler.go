package account

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/dont-wait/anomaly/internal/application/account/commands"
	"github.com/dont-wait/anomaly/internal/application/account/queries"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/domain/audit"
	"github.com/dont-wait/anomaly/internal/infrastructure/rustfs"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
)

const (
	maxAvatarBytes   = 5 << 20
	maxDocumentBytes = 10 << 20
	profileRouteBase = "/api/auth/me"
)

type profileMediaDownloader interface {
	Download(context.Context, string) (*rustfs.DownloadResult, error)
}

type profileUpdateRequest struct {
	Phone   *string                `json:"phone"`
	Email   *string                `json:"email"`
	Address *profileAddressRequest `json:"address"`
}

type profileAddressRequest struct {
	Line         string `json:"line"`
	ProvinceCode string `json:"provinceCode"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

type profileUpload struct {
	data        []byte
	fileName    string
	contentType string
}

func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil || h.updateProfile == nil {
		httpx.WriteError(w, h.logger, middleware.ErrInvalidToken, func(error) int { return http.StatusUnauthorized })
		return
	}
	var req profileUpdateRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, h.logger, err, func(error) int { return http.StatusBadRequest })
		return
	}
	cmd := commands.UpdateCustomerProfileCommand{AccountID: claims.UserID, Phone: req.Phone, Email: req.Email}
	if req.Address != nil {
		cmd.Address = &accountdomain.Address{Line: req.Address.Line, ProvinceCode: req.Address.ProvinceCode}
	}
	account, err := h.updateProfile.Handle(r.Context(), cmd)
	if err != nil {
		httpx.WriteError(w, h.logger, err, accountErrorStatus, accountErrorCode)
		return
	}
	h.recordProfileAudit(r, claims.UserID, "ProfileUpdated")
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteSuccess(w, http.StatusOK, "Profile updated", toProfileResponse(account))
}

func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil || h.changePassword == nil || h.profileRepo == nil {
		httpx.WriteError(w, h.logger, middleware.ErrInvalidToken, func(error) int { return http.StatusUnauthorized })
		return
	}
	var req changePasswordRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, h.logger, err, func(error) int { return http.StatusBadRequest })
		return
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		httpx.WriteError(w, h.logger, accountdomain.ErrWeakPassword, accountErrorStatus, accountErrorCode)
		return
	}
	if err := h.changePassword.Handle(r.Context(), commands.ChangePasswordCommand{AccountID: claims.UserID, CurrentPassword: req.CurrentPassword, NewPassword: req.NewPassword}); err != nil {
		httpx.WriteError(w, h.logger, err, accountErrorStatus, accountErrorCode)
		return
	}
	h.recordProfileAudit(r, claims.UserID, "PasswordChanged")
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteSuccess(w, http.StatusOK, "Password changed", map[string]bool{"changed": true})
}

func (h *Handler) UploadProfileAvatar(w http.ResponseWriter, r *http.Request) {
	h.uploadProfileMedia(w, r, true)
}

func (h *Handler) UploadProfileDocument(w http.ResponseWriter, r *http.Request) {
	h.uploadProfileMedia(w, r, false)
}

func (h *Handler) uploadProfileMedia(w http.ResponseWriter, r *http.Request, avatar bool) {
	w.Header().Set("Cache-Control", "no-store")
	accountID, account, ok := h.currentProfile(w, r)
	if !ok {
		return
	}
	if h.media == nil || h.profileRepo == nil {
		httpx.WriteError(w, h.logger, errors.New("profile media storage unavailable"), func(error) int { return http.StatusServiceUnavailable })
		return
	}
	kind := ""
	limit := int64(maxDocumentBytes)
	if avatar {
		kind, limit = "AVATAR", int64(maxAvatarBytes)
	} else {
		kind = strings.ToUpper(strings.TrimSpace(r.FormValue("type")))
		if kind != "INCOME_PROOF" && kind != "ADDRESS_PROOF" && kind != "IDENTITY_FRONT" && kind != "IDENTITY_BACK" {
			httpx.WriteError(w, h.logger, errors.New("unsupported profile document type"), func(error) int { return http.StatusBadRequest })
			return
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit+(1<<20))
	file, err := readProfileUpload(w, r, "file", limit, avatar)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errProfileFileTooLarge) {
			status = http.StatusRequestEntityTooLarge
		} else if errors.Is(err, errProfileFileType) {
			status = http.StatusUnsupportedMediaType
		}
		httpx.WriteError(w, h.logger, err, func(error) int { return status })
		return
	}
	mediaID := uuid.NewString()
	ext := extensionForMediaType(file.contentType)
	key := fmt.Sprintf("profile/%s/%s%s", accountID, mediaID, ext)
	if err := h.media.Upload(r.Context(), key, strings.NewReader(string(file.data)), file.contentType); err != nil {
		httpx.WriteError(w, h.logger, err, func(error) int { return http.StatusServiceUnavailable })
		return
	}
	media := accountdomain.ProfileMedia{ID: mediaID, Type: kind, FileName: file.fileName, ContentType: file.contentType, StorageKey: key, SizeBytes: int64(len(file.data)), UploadedAt: time.Now().UTC(), Status: "ACTIVE"}
	var previousKey string
	if avatar {
		if account.Customer.Avatar != nil {
			previousKey = account.Customer.Avatar.StorageKey
		}
		account.Customer.Avatar = &media
	} else {
		for _, document := range account.Customer.Documents {
			if document.Type == kind && document.Status == "ACTIVE" && (kind == "IDENTITY_FRONT" || kind == "IDENTITY_BACK") {
				httpx.WriteError(w, h.logger, errors.New("verified identity evidence cannot be replaced here"), func(error) int { return http.StatusForbidden })
				return
			}
		}
		for index := range account.Customer.Documents {
			if account.Customer.Documents[index].Type == kind && account.Customer.Documents[index].Status == "ACTIVE" {
				previousKey = account.Customer.Documents[index].StorageKey
				account.Customer.Documents[index].Status = "REPLACED"
			}
		}
		account.Customer.Documents = append(account.Customer.Documents, media)
	}
	if err := h.saveProfileAggregate(r.Context(), account); err != nil {
		if cleanupErr := h.media.Delete(context.WithoutCancel(r.Context()), key); cleanupErr != nil {
			h.logger.Error().Err(cleanupErr).Msg("delete uncommitted profile media failed")
		}
		httpx.WriteError(w, h.logger, err, accountErrorStatus, accountErrorCode)
		return
	}
	if previousKey != "" {
		if err := h.media.Delete(r.Context(), previousKey); err != nil {
			h.logger.Warn().Err(err).Msg("delete replaced profile media failed")
		}
	}
	h.recordProfileAudit(r, accountID, "ProfileMediaUploaded")
	w.Header().Set("Cache-Control", "no-store")
	response := toProfileMediaResponse(media)
	httpx.WriteSuccess(w, http.StatusCreated, "Profile media uploaded", response)
}

func (h *Handler) ListProfileDocuments(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	_, account, ok := h.currentProfile(w, r)
	if !ok {
		return
	}
	response := toProfileResponse(account)
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteSuccess(w, http.StatusOK, "Profile documents retrieved", response.Documents)
}

func (h *Handler) DeleteProfileDocument(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	accountID, account, ok := h.currentProfile(w, r)
	if !ok {
		return
	}
	if h.media == nil || h.profileRepo == nil {
		httpx.WriteError(w, h.logger, errors.New("profile media storage unavailable"), func(error) int { return http.StatusServiceUnavailable })
		return
	}
	mediaID := r.PathValue("mediaId")
	if mediaID == "kyc-identity-front" || mediaID == "kyc-identity-back" {
		httpx.WriteError(w, h.logger, errors.New("verified identity evidence cannot be deleted here"), func(error) int { return http.StatusForbidden })
		return
	}
	for index := range account.Customer.Documents {
		media := account.Customer.Documents[index]
		if media.ID != mediaID || media.Status != "ACTIVE" {
			continue
		}
		if media.Type == "IDENTITY_FRONT" || media.Type == "IDENTITY_BACK" {
			httpx.WriteError(w, h.logger, errors.New("verified identity evidence cannot be deleted here"), func(error) int { return http.StatusForbidden })
			return
		}
		account.Customer.Documents[index].Status = "DELETED"
		if err := h.saveProfileAggregate(r.Context(), account); err != nil {
			httpx.WriteError(w, h.logger, err, accountErrorStatus, accountErrorCode)
			return
		}
		if err := h.media.Delete(r.Context(), media.StorageKey); err != nil {
			h.logger.Warn().Err(err).Msg("delete profile document object failed")
		}
		h.recordProfileAudit(r, accountID, "ProfileDocumentDeleted")
		httpx.WriteSuccess(w, http.StatusOK, "Profile document deleted", map[string]bool{"deleted": true})
		return
	}
	httpx.WriteError(w, h.logger, accountdomain.ErrAccountNotFound, func(error) int { return http.StatusNotFound })
}

func (h *Handler) DeleteProfileAvatar(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	accountID, account, ok := h.currentProfile(w, r)
	if !ok {
		return
	}
	if h.profileRepo == nil || account.Customer.Avatar == nil || account.Customer.Avatar.Status != "ACTIVE" {
		httpx.WriteError(w, h.logger, accountdomain.ErrAccountNotFound, func(error) int { return http.StatusNotFound })
		return
	}
	previous := account.Customer.Avatar.StorageKey
	account.Customer.Avatar.Status = "DELETED"
	if err := h.saveProfileAggregate(r.Context(), account); err != nil {
		httpx.WriteError(w, h.logger, err, accountErrorStatus, accountErrorCode)
		return
	}
	if h.media != nil {
		if err := h.media.Delete(r.Context(), previous); err != nil {
			h.logger.Warn().Err(err).Msg("delete profile avatar object failed")
		}
	}
	h.recordProfileAudit(r, accountID, "ProfileAvatarDeleted")
	httpx.WriteSuccess(w, http.StatusOK, "Profile avatar deleted", map[string]bool{"deleted": true})
}

func (h *Handler) DownloadProfileMedia(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	_, account, ok := h.currentProfile(w, r)
	if !ok {
		return
	}
	if h.media == nil {
		httpx.WriteError(w, h.logger, errors.New("profile media storage unavailable"), func(error) int { return http.StatusServiceUnavailable })
		return
	}
	mediaID := r.PathValue("mediaId")
	key, contentType, fileName, _, found := profileMediaReference(account, mediaID)
	if !found {
		httpx.WriteError(w, h.logger, rustfs.ErrObjectNotFound, func(error) int { return http.StatusNotFound })
		return
	}
	if contentType != "image/jpeg" && contentType != "image/png" && contentType != "image/webp" && contentType != "application/pdf" && contentType != "video/mp4" && contentType != "video/webm" {
		httpx.WriteError(w, h.logger, errors.New("unsupported stored profile media type"), func(error) int { return http.StatusUnsupportedMediaType })
		return
	}
	downloader, ok := h.media.(profileMediaDownloader)
	if !ok {
		httpx.WriteError(w, h.logger, errors.New("profile media download unavailable"), func(error) int { return http.StatusServiceUnavailable })
		return
	}
	result, err := downloader.Download(r.Context(), key)
	if err != nil {
		httpx.WriteError(w, h.logger, err, func(error) int {
			if errors.Is(err, rustfs.ErrObjectNotFound) {
				return http.StatusNotFound
			}
			return http.StatusServiceUnavailable
		})
		return
	}
	defer result.Body.Close()
	if result.ContentType != "" && contentType == "application/octet-stream" {
		contentType = result.ContentType
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	if strings.HasPrefix(contentType, "image/") || contentType == "application/pdf" {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": fileName}))
	} else {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": fileName}))
	}
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, result.Body); err != nil {
		h.logger.Error().Err(err).Msg("stream profile media failed")
	}
}

func (h *Handler) currentProfile(w http.ResponseWriter, r *http.Request) (string, *accountdomain.UserAccount, bool) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httpx.WriteError(w, h.logger, middleware.ErrInvalidToken, func(error) int { return http.StatusUnauthorized })
		return "", nil, false
	}
	account, err := h.getByID.Handle(r.Context(), queries.GetAccountByIDQuery{ID: claims.UserID})
	if err != nil {
		httpx.WriteError(w, h.logger, err, accountErrorStatus, accountErrorCode)
		return "", nil, false
	}
	if account == nil || account.Customer == nil {
		httpx.WriteError(w, h.logger, accountdomain.ErrAccountNotFound, func(error) int { return http.StatusNotFound })
		return "", nil, false
	}
	return claims.UserID, account, true
}

func (h *Handler) saveProfileAggregate(ctx context.Context, account *accountdomain.UserAccount) error {
	account.Version++
	now := time.Now().UTC()
	account.UpdatedAt = now
	account.Customer.UpdatedAt = now
	return h.profileRepo.Save(ctx, account)
}

func (h *Handler) recordProfileAudit(r *http.Request, accountID, eventType string) {
	if h.profileAudit == nil {
		return
	}
	event := audit.NewEvent(eventType, r.Header.Get("X-Request-ID"), time.Now().UTC())
	event.AccountID, event.ActorID = accountID, accountID
	if err := h.profileAudit.Record(r.Context(), event); err != nil {
		h.logger.Warn().Err(err).Str("event", eventType).Msg("record profile audit event failed")
	}
}

func profileMediaReference(account *accountdomain.UserAccount, mediaID string) (string, string, string, string, bool) {
	if account.Customer.Avatar != nil && account.Customer.Avatar.ID == mediaID && account.Customer.Avatar.Status == "ACTIVE" {
		m := account.Customer.Avatar
		return m.StorageKey, m.ContentType, m.FileName, m.Status, true
	}
	for _, m := range account.Customer.Documents {
		if m.ID == mediaID && m.Status == "ACTIVE" {
			return m.StorageKey, m.ContentType, m.FileName, m.Status, true
		}
	}
	if session := account.VerifiedKYCSession(); session != nil {
		for _, item := range []struct {
			id, name string
			media    accountdomain.MediaObject
		}{
			{"kyc-identity-front", "identity-front", session.Media.IdentityFront},
			{"kyc-identity-back", "identity-back", session.Media.IdentityBack},
			{"kyc-liveness-video", "liveness-video", session.Media.LivenessVideo.MediaObject},
		} {
			if item.id == mediaID && item.media.StorageKey != "" {
				return item.media.StorageKey, item.media.MIMEType, item.name + path.Ext(item.media.StorageKey), "VERIFIED", true
			}
		}
	}
	return "", "", "", "", false
}

var (
	errProfileFileTooLarge = errors.New("profile file exceeds allowed size")
	errProfileFileType     = errors.New("unsupported or mismatched profile file type")
)

func readProfileUpload(w http.ResponseWriter, r *http.Request, field string, limit int64, avatar bool) (*profileUpload, error) {
	r.Body = http.MaxBytesReader(w, r.Body, limit+(1<<20))
	if err := r.ParseMultipartForm(limit); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return nil, errProfileFileTooLarge
		}
		return nil, fmt.Errorf("invalid multipart upload: %w", err)
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile(field)
	if err != nil {
		return nil, fmt.Errorf("missing upload file: %w", err)
	}
	defer file.Close()
	if header.Size <= 0 || header.Size > limit {
		return nil, errProfileFileTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errProfileFileTooLarge
	}
	detected := http.DetectContentType(data)
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		detected = "image/webp"
	}
	allowed := map[string]bool{"image/jpeg": true, "image/png": true}
	if avatar {
		allowed["image/webp"] = true
	} else {
		allowed["application/pdf"] = true
	}
	declared, _, _ := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if !allowed[detected] || (declared != "" && declared != "application/octet-stream" && declared != detected) {
		return nil, errProfileFileType
	}
	if strings.HasPrefix(detected, "image/") && detected != "image/webp" {
		config, format, decodeErr := image.DecodeConfig(bytes.NewReader(data))
		if decodeErr != nil || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 20_000_000 {
			return nil, errProfileFileType
		}
		if (detected == "image/jpeg" && format != "jpeg") || (detected == "image/png" && format != "png") {
			return nil, errProfileFileType
		}
		if _, _, decodeErr := image.Decode(bytes.NewReader(data)); decodeErr != nil {
			return nil, errProfileFileType
		}
	}
	if detected == "image/webp" && !validWebP(data) {
		return nil, errProfileFileType
	}
	if detected == "application/pdf" && !validPDF(data) {
		return nil, errProfileFileType
	}
	name := path.Base(strings.ReplaceAll(header.Filename, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, name)
	if name == "." || name == "/" || name == "" {
		name = "upload" + extensionForMediaType(detected)
	}
	return &profileUpload{data: data, fileName: name, contentType: detected}, nil
}

func validWebP(data []byte) bool {
	return len(data) >= 20 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" && int64(binary.LittleEndian.Uint32(data[4:8]))+8 <= int64(len(data))
}

func validPDF(data []byte) bool {
	return len(data) >= 8 && string(data[:5]) == "%PDF-" && bytes.Contains(data, []byte("%%EOF"))
}

func extensionForMediaType(contentType string) string {
	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "application/pdf":
		return ".pdf"
	default:
		return ".bin"
	}
}
