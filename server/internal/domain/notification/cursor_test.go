package notification

import (
	"errors"
	"testing"
	"time"
)

func TestCursorRoundTrip(t *testing.T) {
	in := Cursor{CreatedAt: time.Date(2026, 11, 4, 8, 0, 0, 123000000, time.UTC), ID: "abc"}
	out, err := DecodeCursor(in.Encode())
	if err != nil || out == nil || !out.CreatedAt.Equal(in.CreatedAt) || out.ID != in.ID {
		t.Fatalf("round trip failed: %+v %v", out, err)
	}
	if c, err := DecodeCursor(""); c != nil || err != nil {
		t.Fatal("empty cursor must decode to nil")
	}
	for _, bad := range []string{"!!", "e30"} { // invalid base64, "{}"
		if _, err := DecodeCursor(bad); !errors.Is(err, ErrInvalidCursor) {
			t.Errorf("%q: expected ErrInvalidCursor, got %v", bad, err)
		}
	}
}
