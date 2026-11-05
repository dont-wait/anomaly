package notification

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Cursor is a position in a recipient's inbox ordered by (created_at, _id).
// Clients treat its encoded form as opaque.
type Cursor struct {
	CreatedAt time.Time `json:"c"`
	ID        string    `json:"i"`
}

var ErrInvalidCursor = fmt.Errorf("%w: cursor", ErrInvalid)

func CursorOf(n Notification) Cursor { return Cursor{CreatedAt: n.CreatedAt, ID: n.ID} }

func (c Cursor) Encode() string {
	raw, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(raw)
}

// DecodeCursor returns nil for an empty value.
func DecodeCursor(value string) (*Cursor, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, ErrInvalidCursor
	}
	var cursor Cursor
	if err := json.Unmarshal(raw, &cursor); err != nil || cursor.ID == "" || cursor.CreatedAt.IsZero() {
		return nil, ErrInvalidCursor
	}
	return &cursor, nil
}
