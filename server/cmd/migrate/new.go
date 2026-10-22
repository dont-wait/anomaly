package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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
	upName := base + ".up.json"
	downName := base + ".down.json"
	for _, name := range []string{upName, downName} {
		if _, exists := seen[name]; exists {
			return "", "", fmt.Errorf("migration %q already exists", filepath.Join(dir, name))
		}
	}

	upPath := filepath.Join(dir, upName)
	downPath := filepath.Join(dir, downName)
	if err := os.WriteFile(upPath, []byte("[]\n"), 0o644); err != nil {
		return "", "", fmt.Errorf("write up migration: %w", err)
	}
	if err := os.WriteFile(downPath, []byte("[]\n"), 0o644); err != nil {
		_ = os.Remove(upPath)
		return "", "", fmt.Errorf("write down migration: %w", err)
	}
	return upPath, downPath, nil
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
