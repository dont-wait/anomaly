package rustfs

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain"
)

func setupIntegrationRepo(t *testing.T) *MediaRepository {
	t.Helper()

	loader := domain.GetEnvLoader().Load(nil)
	conf := loader.LoadRustFSConfig()

	client := NewClient(conf)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := EnsureBucket(ctx, client, conf.Bucket); err != nil {
		t.Skipf("rustfs not reachable, skipping integration test: %v", err)
	}

	return NewMediaRepository(client, conf.Bucket)
}

func TestUploadGetDeleteEndToEnd(t *testing.T) {
	repo := setupIntegrationRepo(t)
	ctx := context.Background()

	key := "test/integration-" + time.Now().Format("20060102150405") + ".txt"
	content := "hello rustfs integration test"
	file := createTempUploadFile(t, content)
	defer file.Close()

	if err := repo.Upload(ctx, key, file, "text/plain"); err != nil {
		t.Fatalf("Upload() error = %v, want nil", err)
	}

	result, err := repo.Download(ctx, key)
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	data, readErr := io.ReadAll(result.Body)
	result.Body.Close()
	if readErr != nil {
		t.Fatalf("read body error = %v", readErr)
	}
	if string(data) != content {
		t.Fatalf("Get() content = %q, want %q", string(data), content)
	}
	if result.ContentType != "text/plain" {
		t.Fatalf("Get() content type = %q, want %q", result.ContentType, "text/plain")
	}

	if err := repo.Delete(ctx, key); err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}

	_, err = repo.Download(ctx, key)
	if !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("Get() after delete error = %v, want ErrObjectNotFound", err)
	}
}
