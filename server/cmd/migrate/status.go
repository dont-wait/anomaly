package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source"
)

type migrationEntry struct {
	version uint
	name    string
}

func listMigrations(src source.Driver) ([]migrationEntry, error) {
	version, err := src.First()
	if errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("no migration files found in migrations directory")
	}
	if err != nil {
		return nil, err
	}
	var entries []migrationEntry
	for {
		body, name, err := src.ReadUp(version)
		if err != nil {
			return nil, fmt.Errorf("read up migration %d: %w", version, err)
		}
		if err := body.Close(); err != nil {
			return nil, err
		}
		entries = append(entries, migrationEntry{version, name})
		version, err = src.Next(version)
		if errors.Is(err, os.ErrNotExist) {
			return entries, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

type versionReader interface {
	Version() (uint, bool, error)
}

func printStatus(w io.Writer, runner versionReader, entries []migrationEntry) error {
	version, dirty, err := runner.Version()
	hasVersion := !errors.Is(err, migrate.ErrNilVersion)
	if err != nil && hasVersion {
		return err
	}
	if hasVersion {
		if _, err := fmt.Fprintf(w, "version=%d dirty=%t\n", version, dirty); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(w, "version=none dirty=false"); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "STATUS\tVERSION\tMIGRATION"); err != nil {
		return err
	}
	found := false
	for _, entry := range entries {
		state := "pending"
		if hasVersion && entry.version <= version {
			state = "applied"
		}
		if hasVersion && entry.version == version {
			found = true
			if dirty {
				state = "dirty"
			}
		} else if dirty {
			// A failed down migration can leave the target version dirty too.
			state = "unknown"
		}
		if _, err := fmt.Fprintf(w, "%s\t%d\t%s\n", state, entry.version, entry.name); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "Applied status is inferred from the stored version; no per-file history or schema comparison is available."); err != nil {
		return err
	}
	if dirty {
		if _, err := fmt.Fprintln(w, "Dirty state: a migration did not complete; other migration states cannot be confirmed."); err != nil {
			return err
		}
	}
	if hasVersion && !found {
		if _, err := fmt.Fprintln(w, "Warning: stored version has no matching local migration; check the database and migrations directory."); err != nil {
			return err
		}
	}
	return nil
}
