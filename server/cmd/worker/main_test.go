package main

import (
	"errors"
	"testing"
)

func TestPermanentProjectionErrorPreservesClassificationAndCause(t *testing.T) {
	cause := errors.New("duplicate key")
	err := &permanentProjectionError{
		reason: "duplicate_key",
		err:    cause,
	}

	var permanentErr *permanentProjectionError
	if !errors.As(err, &permanentErr) {
		t.Fatal("errors.As() = false, want true")
	}
	if permanentErr.reason != "duplicate_key" {
		t.Fatalf("reason = %q, want duplicate_key", permanentErr.reason)
	}
	if !errors.Is(err, cause) {
		t.Fatal("errors.Is() = false, want wrapped cause")
	}
}
