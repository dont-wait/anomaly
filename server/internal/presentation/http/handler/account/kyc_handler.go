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
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/dont-wait/anomaly/internal/application/account/commands"
	"github.com/dont-wait/anomaly/internal/application/account/queries"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	kycinfra "github.com/dont-wait/anomaly/internal/infrastructure/kyc"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
)

const (
	maxKYCRequestBytes = 48 << 20
	maxImageBytes      = 10 << 20
	maxVideoBytes      = 25 << 20
)

type kycUpload struct {
	filename    string
	contentType string
	data        []byte
	extension   string
}

func (h *Handler) CompleteKYC(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httpx.WriteError(w, h.logger, middleware.ErrInvalidToken, func(error) int { return http.StatusUnauthorized })
		return
	}

	account, err := h.getByID.Handle(r.Context(), queries.GetAccountByIDQuery{ID: claims.UserID})
	if err != nil {
		httpx.WriteError(w, h.logger, err, accountErrorStatus, accountErrorCode)
		return
	}
	if account.IsVerified() {
		writeVerifiedKYC(w, account)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxKYCRequestBytes)
	if err := r.ParseMultipartForm(maxKYCRequestBytes); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			httpx.WriteError(w, h.logger, errors.New("KYC payload too large"), func(error) int { return http.StatusRequestEntityTooLarge }, func(error) httpx.ErrorCode { return httpx.ErrorCodePayloadTooLarge })
			return
		}
		httpx.WriteError(w, h.logger, fmt.Errorf("invalid multipart request: %w", err), func(error) int { return http.StatusBadRequest })
		return
	}
	defer r.MultipartForm.RemoveAll()

	challengeType := strings.TrimSpace(r.FormValue("challengeType"))
	if challengeType == "" {
		httpx.WriteError(w, h.logger, errors.New("challengeType is required"), func(error) int { return http.StatusBadRequest })
		return
	}
	front, err := readKYCUpload(r.MultipartForm, "idCardFront", maxImageBytes, true)
	if err != nil {
		httpx.WriteError(w, h.logger, err, func(error) int { return http.StatusBadRequest })
		return
	}
	back, err := readKYCUpload(r.MultipartForm, "idCardBack", maxImageBytes, true)
	if err != nil {
		httpx.WriteError(w, h.logger, err, func(error) int { return http.StatusBadRequest })
		return
	}
	video, err := readKYCUpload(r.MultipartForm, "liveVideo", maxVideoBytes, false)
	if err != nil {
		httpx.WriteError(w, h.logger, err, func(error) int { return http.StatusBadRequest })
		return
	}

	if h.kyc == nil || h.media == nil {
		h.logger.Error().Msg("KYC dependencies are unavailable")
		writeKYCSystemError(w, "KYC verification is temporarily unavailable")
		return
	}
	result, err := h.kyc.VerifyFace(r.Context(),
		kycinfra.Media{Filename: front.filename, ContentType: front.contentType, Data: front.data},
		kycinfra.Media{Filename: video.filename, ContentType: video.contentType, Data: video.data},
		challengeType,
	)
	if err != nil {
		h.logger.Error().Err(err).Msg("KYC service verification failed")
		writeKYCSystemError(w, "KYC verification is temporarily unavailable")
		return
	}
	if result.Decision == "VERIFIED" && !result.Success {
		h.logger.Error().Msg("KYC verifier returned an inconsistent VERIFIED result")
		writeKYCSystemError(w, "KYC verification is temporarily unavailable")
		return
	}
	if result.Decision != "VERIFIED" {
		httpx.WriteSuccess(w, http.StatusOK, "KYC verification completed", KYCCompleteResponse{
			Decision: result.Decision, ReasonCode: result.ReasonCode, ReasonMessage: result.ReasonMessage,
		})
		return
	}

	// Avoid duplicate official uploads if another completion finished during verification.
	account, err = h.getByID.Handle(r.Context(), queries.GetAccountByIDQuery{ID: claims.UserID})
	if err != nil {
		httpx.WriteError(w, h.logger, err, accountErrorStatus, accountErrorCode)
		return
	}
	if account.IsVerified() {
		writeVerifiedKYC(w, account)
		return
	}

	keys := []string{
		fmt.Sprintf("kyc/%s/id-card-front/%s%s", claims.UserID, uuid.NewString(), front.extension),
		fmt.Sprintf("kyc/%s/id-card-back/%s%s", claims.UserID, uuid.NewString(), back.extension),
		fmt.Sprintf("kyc/%s/live-video/%s%s", claims.UserID, uuid.NewString(), video.extension),
	}
	uploads := []kycUpload{front, back, video}
	uploaded := 0
	for i := range uploads {
		if err := h.media.Upload(r.Context(), keys[i], bytes.NewReader(uploads[i].data), uploads[i].contentType); err != nil {
			h.logger.Error().Err(err).Msg("store verified KYC media failed")
			deleteMedia(r.Context(), h.media, keys[:uploaded], h.logger)
			writeKYCSystemError(w, "KYC media storage is temporarily unavailable")
			return
		}
		uploaded++
	}

	account, err = h.verify.Handle(r.Context(), commands.VerifyAccountCommand{
		AccountID: claims.UserID, IdCardFrontUrl: keys[0], IdCardBackUrl: keys[1], LiveVideoUrl: keys[2],
	})
	if err != nil {
		deleteMedia(r.Context(), h.media, keys, h.logger)
		httpx.WriteError(w, h.logger, err, accountErrorStatus, accountErrorCode)
		return
	}
	writeVerifiedKYC(w, account)
}

func readKYCUpload(form *multipart.Form, field string, maxBytes int64, imageFile bool) (kycUpload, error) {
	headers := form.File[field]
	if len(headers) != 1 {
		return kycUpload{}, fmt.Errorf("%s must contain exactly one file", field)
	}
	header := headers[0]
	file, err := header.Open()
	if err != nil {
		return kycUpload{}, fmt.Errorf("open %s: %w", field, err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return kycUpload{}, fmt.Errorf("read %s: %w", field, err)
	}
	if len(data) == 0 || int64(len(data)) > maxBytes {
		return kycUpload{}, fmt.Errorf("%s is empty or too large", field)
	}
	declared, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		return kycUpload{}, fmt.Errorf("%s has an invalid content type", field)
	}
	actual, extension, err := validateMedia(data, imageFile)
	if err != nil {
		return kycUpload{}, fmt.Errorf("%s: %w", field, err)
	}
	if declared != actual {
		return kycUpload{}, fmt.Errorf("%s content type does not match its contents", field)
	}
	return kycUpload{filename: filepath.Base(header.Filename), contentType: actual, data: data, extension: extension}, nil
}

func validateMedia(data []byte, imageFile bool) (string, string, error) {
	if imageFile {
		config, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return "", "", errors.New("file is not a valid JPEG or PNG image")
		}
		if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 20_000_000 {
			return "", "", errors.New("image dimensions are invalid or too large")
		}
		if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
			return "", "", errors.New("image data is corrupt")
		}
		switch format {
		case "jpeg":
			return "image/jpeg", ".jpg", nil
		case "png":
			return "image/png", ".png", nil
		default:
			return "", "", errors.New("image format is not supported")
		}
	}
	if len(data) >= 16 && string(data[4:8]) == "ftyp" {
		boxSize := int(binary.BigEndian.Uint32(data[:4]))
		brand := string(data[8:12])
		if boxSize >= 16 && boxSize <= len(data) && validMP4Brand(brand) {
			return "video/mp4", ".mp4", nil
		}
	}
	if len(data) >= 12 && bytes.Equal(data[:4], []byte{0x1a, 0x45, 0xdf, 0xa3}) {
		probe := data
		if len(probe) > 4096 {
			probe = probe[:4096]
		}
		if bytes.Contains(bytes.ToLower(probe), []byte("webm")) {
			return "video/webm", ".webm", nil
		}
	}
	return "", "", errors.New("file is not a valid MP4 or WebM video")
}

func validMP4Brand(brand string) bool {
	switch brand {
	case "isom", "iso2", "iso4", "iso5", "iso6", "mp41", "mp42", "avc1", "M4V ", "qt  ":
		return true
	default:
		return false
	}
}

func deleteMedia(ctx context.Context, store MediaStore, keys []string, logger zerolog.Logger) {
	for _, key := range keys {
		if err := store.Delete(ctx, key); err != nil {
			logger.Error().Err(err).Str("key", key).Msg("roll back KYC media failed")
		}
	}
}

func writeVerifiedKYC(w http.ResponseWriter, account *accountdomain.UserAccount) {
	user := toAccountResponsePrivate(account)
	httpx.WriteSuccess(w, http.StatusOK, "KYC verification completed", KYCCompleteResponse{Decision: "VERIFIED", User: &user})
}

func writeKYCSystemError(w http.ResponseWriter, message string) {
	httpx.WriteSuccess(w, http.StatusBadGateway, "KYC verification failed", KYCCompleteResponse{Decision: "SYSTEM_ERROR", ReasonMessage: message})
}
