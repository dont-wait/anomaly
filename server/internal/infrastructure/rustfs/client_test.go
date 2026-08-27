package rustfs

import (
	"context"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain"
)

func TestEnsureBucketReturnsErrorWhenRustFSUnavailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	conf := &domain.RustFSConfig{
		Endpoint:  "http://127.0.0.1:1",
		AccessKey: "test",
		SecretKey: "test",
		Region:    "us-east-1",
	}

	client := NewClient(conf)

	err := EnsureBucket(ctx, client, "media")
	if err == nil {
		t.Fatal("EnsureBucket() error = nil, want non-nil")
	}
}
