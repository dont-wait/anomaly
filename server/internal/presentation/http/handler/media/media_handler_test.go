package media_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"

	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/infrastructure/rustfs"
	"github.com/dont-wait/anomaly/internal/presentation/http/handler/media"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
)

func TestUploadUsesSuccessEnvelope(t *testing.T) {
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("storage method = %s, want PUT", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer storage.Close()

	client := rustfs.NewClient(&domain.RustFSConfig{
		Endpoint:  storage.URL,
		AccessKey: "test-access",
		SecretKey: "test-secret",
		Bucket:    "media",
		Region:    "us-east-1",
	})
	handler := media.NewHandler(
		zerolog.Nop(),
		rustfs.NewMediaRepository(client, "media"),
	)

	body, contentType := multipartBody(t, true)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/media/upload", body)
	request.Header.Set("Content-Type", contentType)
	handler.Upload(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", recorder.Code)
	}
	var response httpx.SuccessResponse[map[string]string]
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != http.StatusCreated {
		t.Fatalf("body status = %d, want 201", response.Status)
	}
	if response.Data["key"] != "kyc/front.jpg" {
		t.Fatalf("data = %#v, want uploaded key", response.Data)
	}
}

func TestUploadMissingKeyUsesErrorEnvelope(t *testing.T) {
	body, contentType := multipartBody(t, false)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/media/upload", body)
	request.Header.Set("Content-Type", contentType)

	media.NewHandler(zerolog.Nop(), nil).Upload(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	var response httpx.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != http.StatusBadRequest || len(response.Errors) != 1 {
		t.Fatalf("error response = %#v, want bad request envelope", response)
	}
	if response.Errors[0].Code != httpx.ErrorCodeMissingKey {
		t.Fatalf("error code = %q, want %q", response.Errors[0].Code, httpx.ErrorCodeMissingKey)
	}
}

func multipartBody(t *testing.T, includeKey bool) (io.Reader, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if includeKey {
		if err := writer.WriteField("key", "kyc/front.jpg"); err != nil {
			t.Fatalf("write key: %v", err)
		}
	}
	file, err := writer.CreateFormFile("file", "front.jpg")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := file.Write([]byte("image")); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return &body, writer.FormDataContentType()
}
