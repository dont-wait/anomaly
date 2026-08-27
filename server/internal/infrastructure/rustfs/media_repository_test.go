package rustfs

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain"
)

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

func TestUploadReturnsErrorWhenRustFSUnavailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	repo := newUnreachableRepo(t)

	err := repo.Upload(ctx, "test/key.txt", strings.NewReader("hello"), "text/plain")
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

func TestDeleteReturnsErrorWhenRustFSUnavailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	repo := newUnreachableRepo(t)

	err := repo.Delete(ctx, "test/key.txt")
	if err == nil {
		t.Fatal("Delete() error = nil, want non-nil")
	}
}
