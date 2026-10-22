package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateMigrationUsesNextVersion(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "000005_existing.up.json")
	if err := os.WriteFile(existing, []byte("[]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	upPath, downPath, err := createMigration(dir, "Add Loan Repayment")
	if err != nil {
		t.Fatal(err)
	}

	if want := filepath.Join(dir, "000006_add_loan_repayment.up.json"); upPath != want {
		t.Fatalf("up path = %q, want %q", upPath, want)
	}
	if want := filepath.Join(dir, "000006_add_loan_repayment.down.json"); downPath != want {
		t.Fatalf("down path = %q, want %q", downPath, want)
	}
	for _, path := range []string{upPath, downPath} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != "[]\n" {
			t.Fatalf("%s content = %q, want empty JSON array", path, body)
		}
	}
}

func TestCreateMigrationStartsAtOne(t *testing.T) {
	dir := t.TempDir()

	upPath, _, err := createMigration(dir, "create_customers")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "000001_create_customers.up.json"); upPath != want {
		t.Fatalf("up path = %q, want %q", upPath, want)
	}
}

func TestCreateMigrationRejectsInvalidName(t *testing.T) {
	if _, _, err := createMigration(t.TempDir(), "  ---  "); err == nil {
		t.Fatal("expected invalid name error")
	}
}

func TestCreateMigrationRequiresDirectory(t *testing.T) {
	if _, _, err := createMigration(filepath.Join(t.TempDir(), "missing"), "test"); err == nil {
		t.Fatal("expected missing directory error")
	}
}
