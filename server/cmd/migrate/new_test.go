package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
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

	upPath, downPath, err := createMigration(dir, "create_customers")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "000001_create_customers.up.json"); upPath != want {
		t.Fatalf("up path = %q, want %q", upPath, want)
	}
	if want := filepath.Join(dir, "000001_create_customers.down.json"); downPath != want {
		t.Fatalf("down path = %q, want %q", downPath, want)
	}
	for _, path := range []string{upPath, downPath} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if string(body) != "[]\n" {
			t.Fatalf("%s content = %q, want empty JSON array", path, body)
		}
	}
}

func TestWriteMigrationFileNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "000001_create_customers.up.json")
	if err := os.WriteFile(path, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := writeMigrationFile(path); err == nil {
		t.Fatal("expected existing file error")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "keep\n" {
		t.Fatalf("existing migration was overwritten: %q", body)
	}
}

func TestCreateMigrationSerializesConcurrentCreates(t *testing.T) {
	dir := t.TempDir()
	const total = 6

	var wg sync.WaitGroup
	errs := make(chan error, total)
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, _, err := createMigration(dir, fmt.Sprintf("migration_%d", i)); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("createMigration: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, migrationLockName)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("migration lock was not removed: %v", err)
	}
	for version := 1; version <= total; version++ {
		matches, err := filepath.Glob(filepath.Join(dir, fmt.Sprintf("%06d_*.up.json", version)))
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 1 {
			t.Fatalf("version %d has %d up migrations, want 1", version, len(matches))
		}
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
