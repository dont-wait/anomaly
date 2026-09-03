package media

import (
	"errors"
	"io"
	"net/http"

	"github.com/rs/zerolog"

	"github.com/dont-wait/anomaly/internal/infrastructure/rustfs"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
)

// giới hạn kích thước
const maxUploadBytes = 32 << 20

// khai báo
type Handler struct {
	logger zerolog.Logger          // ghi log lỗi
	repo   *rustfs.MediaRepository // công cụ Upload/Download
}

// nhận log và repo và tra về handler
func NewHandler(logger zerolog.Logger, repo *rustfs.MediaRepository) *Handler {
	return &Handler{logger: logger, repo: repo}
}

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) { // gọi w là nơi để viết repo và trả về cho client
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes) // nếu request vượt maxUploadBytes, việc đọc body sẽ tự dừng và trả lỗi.

	if err := r.ParseMultipartForm(maxUploadBytes); err != nil { // đọc và bóc tách request đó ra thành từng phần riêng biệt, giới hạn kích thước
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			httpx.WriteJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "file too large"})
			return
		}
		httpx.WriteError(w, h.logger, err, func(error) int { return http.StatusBadRequest })
		return
	}
	key := r.FormValue("key") // lấy giá trị của field
	if key == "" {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "missing key"})
		return
	}

	file, header, err := r.FormFile("file") // lấy ra file đính kèm trong field và trả về 3 giá trị file, header, err
	if err != nil {
		httpx.WriteError(w, h.logger, err, func(error) int { return http.StatusBadRequest })
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			h.logger.Warn().Err(err).Msg("close upload file failed")
		}
	}()

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	//  gọi thẳng tới MediaRepository.Upload(), chuyển 3 giá trị cho nó xử lí
	if err := h.repo.Upload(r.Context(), key, file, contentType); err != nil {
		httpx.WriteError(w, h.logger, err, func(error) int { return http.StatusInternalServerError })
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]string{"key": key}) // thành công thì trả về
}

// Download lấy từ URL vì đây là request GET (không có body chứa form data như POST)
func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "missing key"})
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
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, result.Body); err != nil {
		h.logger.Error().Err(err).Msg("stream download response failed")
	}
}
