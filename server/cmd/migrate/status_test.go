package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

type stubVersion struct {
	version uint
	dirty   bool
	err     error
}

func (s stubVersion) Version() (uint, bool, error) { return s.version, s.dirty, s.err }

func TestPrintStatus(t *testing.T) {
	entries := []migrationEntry{{1, "customers"}, {2, "kyc"}, {3, "accounts"}}
	for _, tt := range []struct {
		name  string
		state stubVersion
		want  []string
	}{
		{"fresh", stubVersion{err: migrate.ErrNilVersion}, []string{"version=none", "pending\t1", "pending\t3"}},
		{"partial", stubVersion{version: 2}, []string{"applied\t1", "applied\t2", "pending\t3"}},
		{"latest", stubVersion{version: 3}, []string{"applied\t3"}},
		{"dirty", stubVersion{version: 2, dirty: true}, []string{"unknown\t1", "dirty\t2", "unknown\t3"}},
		{"missing local", stubVersion{version: 4}, []string{"Warning: stored version has no matching local migration"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := printStatus(&out, tt.state, entries); err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.want {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("output %q missing %q", out.String(), want)
				}
			}
		})
	}
	wantErr := errors.New("database unavailable")
	if err := printStatus(&bytes.Buffer{}, stubVersion{err: wantErr}, entries); !errors.Is(err, wantErr) {
		t.Fatalf("got %v", err)
	}
}

func TestListMigrations(t *testing.T) {
	src, err := iofs.New(os.DirFS("../../migrations"), ".")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := src.Close(); err != nil {
			t.Errorf("close migration source: %v", err)
		}
	})
	entries, err := listMigrations(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || entries[0].name != "create_customers" || entries[2].version != 3 {
		t.Fatalf("unexpected migrations: %+v", entries)
	}
}

func TestListMigrationsRejectsEmptyOrMissingUp(t *testing.T) {
	for _, fs := range []fstest.MapFS{
		{"README.md": &fstest.MapFile{Data: []byte("not a migration")}},
		{"000001_only.down.json": &fstest.MapFile{Data: []byte("[]")}},
	} {
		src, err := iofs.New(fs, ".")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := listMigrations(src); err == nil {
			t.Fatal("expected source validation error")
		}
		if err := src.Close(); err != nil {
			t.Fatalf("close migration source: %v", err)
		}
	}
}

// failAfterWriter fails after a chosen number of bytes have been written.
type failAfterWriter struct {
	remaining int
	err       error
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if len(p) > w.remaining {
		n := w.remaining
		w.remaining = 0
		return n, w.err
	}
	w.remaining -= len(p)
	return len(p), nil
}

func TestPrintStatusReturnsWriteErrors(t *testing.T) {
	entries := []migrationEntry{{1, "customers"}}
	for _, state := range []stubVersion{
		{err: migrate.ErrNilVersion},
		{version: 1},
		{version: 2, dirty: true},
	} {
		var output bytes.Buffer
		if err := printStatus(&output, state, entries); err != nil {
			t.Fatal(err)
		}
		for limit := 0; limit < output.Len(); limit++ {
			wantErr := errors.New("output unavailable")
			writer := &failAfterWriter{remaining: limit, err: wantErr}
			if err := printStatus(writer, state, entries); !errors.Is(err, wantErr) {
				t.Fatalf("state %+v, byte %d: got %v, want %v", state, limit, err, wantErr)
			}
		}
	}
}
