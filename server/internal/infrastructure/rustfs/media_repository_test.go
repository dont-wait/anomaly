package rustfs

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain"
)

func createTempUploadFile(t *testing.T, content string) *os.File {
	t.Helper()

	path := filepath.Join(t.TempDir(), "upload.txt")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("os.Open() error = %v", err)
	}

	return file
}

func newUnreachableRepo(t *testing.T) *MediaRepository {
	t.Helper()
	conf := &domain.RustFSConfig{
		Endpoint:  "http://127.0.0.1:1",
		AccessKey: "test",
		SecretKey: "test",
		Region:    "us-east-1",
	}
	client := NewClient(conf)
	return NewMediaRepository(client, "media")
}

func newTestRepoWithServer(t *testing.T, handler http.HandlerFunc) *MediaRepository {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	conf := &domain.RustFSConfig{
		Endpoint:  server.URL,
		AccessKey: "test",
		SecretKey: "test",
		Region:    "us-east-1",
	}

	return NewMediaRepository(NewClient(conf), "media")
}

func TestUploadReturnsErrorWhenRustFSUnavailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	repo := newUnreachableRepo(t)
	file := createTempUploadFile(t, "hello")
	defer file.Close()

	err := repo.Upload(ctx, "test/key.txt", file, "text/plain")
	if err == nil {
		t.Fatal("Upload() error = nil, want non-nil")
	}
}

func TestDownloadReturnsErrorWhenRustFSUnavailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	repo := newUnreachableRepo(t)

	_, err := repo.Download(ctx, "test/key.txt")
	if err == nil {
		t.Fatal("Download() error = nil, want non-nil")
	}
}

func TestDownloadReturnsObjectNotFoundWhenKeyDoesNotExist(t *testing.T) {
	repo := newTestRepoWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("request method = %q, want %q", r.Method, http.MethodGet)
		}

		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message><Key>test/key.txt</Key></Error>`))
	})

	_, err := repo.Download(context.Background(), "test/key.txt")
	if !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("Download() error = %v, want ErrObjectNotFound", err)
	}
}

func TestDownloadFallsBackToOctetStreamWhenContentTypeMissing(t *testing.T) {
	repo := newTestRepoWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("request method = %q, want %q", r.Method, http.MethodGet)
		}

		w.WriteHeader(http.StatusOK)
	})

	result, err := repo.Download(context.Background(), "test/key.txt")
	if err != nil {
		t.Fatalf("Download() error = %v, want nil", err)
	}
	defer result.Body.Close()

	if result.ContentType != "application/octet-stream" {
		t.Fatalf("Download() content type = %q, want %q", result.ContentType, "application/octet-stream")
	}
}

func TestDeleteReturnsErrorWhenRustFSUnavailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	repo := newUnreachableRepo(t)

	err := repo.Delete(ctx, "test/key.txt")
	if err == nil {
		t.Fatal("Delete() error = nil, want non-nil")
	}
}
