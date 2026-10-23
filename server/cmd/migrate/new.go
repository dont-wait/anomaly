package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// The lock file does not match migrationFilePattern, so the migration
	// source and the migration tests both ignore it.
	migrationLockName    = ".migrate-new.lock"
	migrationLockWait    = 5 * time.Second
	migrationLockRetry   = 50 * time.Millisecond
	migrationFileContent = "[]\n"
)

var (
	migrationFilePattern = regexp.MustCompile(`^(\d+)_(.+)\.(up|down)\.json$`)
	invalidNamePattern   = regexp.MustCompile(`[^a-z0-9]+`)
)

// createMigration writes empty up/down migration files using the next
// available version number. Existing migrations are never overwritten.
func createMigration(dir, name string) (string, string, error) {
	slug, err := migrationSlug(name)
	if err != nil {
		return "", "", err
	}
	if info, err := os.Stat(dir); err != nil {
		return "", "", fmt.Errorf("read migrations directory: %w", err)
	} else if !info.IsDir() {
		return "", "", fmt.Errorf("migrations path %q is not a directory", dir)
	}
	release, err := lockMigrations(dir)
	if err != nil {
		return "", "", err
	}
	defer release()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", "", fmt.Errorf("read migrations directory: %w", err)
	}

	var maxVersion uint
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		seen[entry.Name()] = struct{}{}
		matches := migrationFilePattern.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue
		}
		version, err := strconv.ParseUint(matches[1], 10, 32)
		if err != nil {
			return "", "", fmt.Errorf("parse migration version in %q: %w", entry.Name(), err)
		}
		if uint(version) > maxVersion {
			maxVersion = uint(version)
		}
	}

	version := maxVersion + 1
	base := fmt.Sprintf("%06d_%s", version, slug)
	upPath := filepath.Join(dir, base+".up.json")
	downPath := filepath.Join(dir, base+".down.json")
	for _, path := range []string{upPath, downPath} {
		if _, exists := seen[filepath.Base(path)]; exists {
			return "", "", fmt.Errorf("migration %q already exists", path)
		}
	}

	// Create the down file first: if the process is interrupted between the
	// two writes, the leftover down-only file is ignored by migration tooling
	// and by the migration tests, while an up-only file would look runnable
	// but could not be rolled back.
	if err := writeMigrationFile(downPath); err != nil {
		return "", "", err
	}
	if err := writeMigrationFile(upPath); err != nil {
		_ = os.Remove(downPath)
		return "", "", err
	}
	return upPath, downPath, nil
}

// writeMigrationFile creates the file exclusively so concurrent runs can never
// truncate or overwrite a migration that another process just created.
func writeMigrationFile(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("migration %q already exists", path)
		}
		return fmt.Errorf("create migration %q: %w", path, err)
	}
	if _, err := file.WriteString(migrationFileContent); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("write migration %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("close migration %q: %w", path, err)
	}
	return nil
}

// lockMigrations serializes version selection and file creation so concurrent
// runs cannot allocate the same version.
func lockMigrations(dir string) (func(), error) {
	lockPath := filepath.Join(dir, migrationLockName)
	deadline := time.Now().Add(migrationLockWait)
	for {
		file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_ = file.Close()
			return func() { _ = os.Remove(lockPath) }, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("create migration lock: %w", err)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("migrations directory is locked by another migrate new; remove %s if stale", lockPath)
		}
		time.Sleep(migrationLockRetry)
	}
}

func migrationSlug(name string) (string, error) {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = invalidNamePattern.ReplaceAllString(slug, "_")
	slug = strings.Trim(slug, "_")
	if slug == "" {
		return "", errors.New("migration name must contain at least one letter or digit")
	}
	return slug, nil
}
