package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateEnvironmentRequiresLocalExplicitOptIn(t *testing.T) {
	tests := []struct {
		name  string
		env   string
		allow string
		conn  string
		want  bool
	}{
		{name: "local target", env: "development", allow: "true", conn: "esdb://localhost:2113?tls=false", want: true},
		{name: "test environment", env: "test", allow: "true", conn: "esdb://localhost:2113?tls=false", want: true},
		{name: "missing opt in", env: "development", allow: "false", conn: "esdb://localhost:2113?tls=false", want: false},
		{name: "remote target", env: "development", allow: "true", conn: "esdb://prod.example:2113?tls=true", want: false},
		{name: "credentials", env: "development", allow: "true", conn: "esdb://user:secret@localhost:2113?tls=false", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("APP_ENV", test.env)
			t.Setenv("SEED_DEMO_ENABLED", "true")
			t.Setenv("SEED_ANOMALY_ALLOW_TARGET", test.allow)
			err := validateEnvironment(test.conn)
			if (err == nil) != test.want {
				t.Fatalf("validateEnvironment() error = %v, want success=%v", err, test.want)
			}
		})
	}
}

func TestWriteManifestRestrictsExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(path, "dataset", "in_progress", nil); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("manifest mode = %o, want 600", mode)
	}
}

func TestValidateDatasetRejectsUnsupportedSchema(t *testing.T) {
	if err := validateDataset(dataset{SchemaVersion: 2, DatasetID: "dataset", Accounts: []fixtureAccount{{ID: "a", Kind: "receiver", CCCD: "012345678901"}}}); err == nil {
		t.Fatal("validateDataset() error = nil, want unsupported schema error")
	}
}
