package mongo

import (
	"context"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain"
)

func TestNewMongoClientReturnsErrorWhenMongoUnavailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	conf := &domain.MongoConfig{
		MongoURI: "mongodb://127.0.0.1:1/?serverSelectionTimeoutMS=50&connectTimeoutMS=50",
	}

	got, err := NewMongoClient(ctx, conf)
	if err == nil {
		t.Fatal("NewMongoClient() error = nil, want non-nil")
	}
	if got != nil {
		t.Fatalf("NewMongoClient() client = %#v, want nil", got)
	}
}
