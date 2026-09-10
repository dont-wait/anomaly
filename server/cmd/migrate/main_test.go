package main

import (
	"testing"

	"github.com/dont-wait/anomaly/internal/domain"
)

func TestMongoDatabaseURL(t *testing.T) {
	t.Setenv("MONGO_URI", "mongodb://user:password@localhost:27017/?authSource=admin")
	t.Setenv("MONGO_DB", "migration_test")

	got, err := mongoDatabaseURL(domain.GetEnvLoader())
	if err != nil {
		t.Fatalf("mongoDatabaseURL() error = %v", err)
	}
	const want = "mongodb://user:password@localhost:27017/migration_test?authSource=admin"
	if got != want {
		t.Fatalf("mongoDatabaseURL() = %q, want %q", got, want)
	}
}

func TestMongoDatabaseURLRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name     string
		mongoURI string
		database string
	}{
		{name: "unsupported scheme", mongoURI: "postgres://localhost", database: "migration_test"},
		{name: "missing database", mongoURI: "mongodb://localhost:27017", database: " "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("MONGO_URI", tt.mongoURI)
			t.Setenv("MONGO_DB", tt.database)

			if _, err := mongoDatabaseURL(domain.GetEnvLoader()); err == nil {
				t.Fatal("mongoDatabaseURL() error = nil, want error")
			}
		})
	}
}
