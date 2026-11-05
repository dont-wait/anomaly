package notification

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/notification"
)

const (
	DefaultListLimit = 20
	MaxListLimit     = 50
)

// Recipient always comes from authenticated claims, never from client input.
type Recipient struct {
	Audience notification.Audience
	ID       string
}

func (r Recipient) Key() string { return notification.RecipientKey(r.Audience, r.ID) }

func (r Recipient) validate() error {
	if !r.Audience.Valid() || strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("%w: recipient", notification.ErrInvalid)
	}
	return nil
}

type ListQuery struct {
	Recipient  Recipient
	Cursor     string
	Limit      int
	UnreadOnly bool
}

type Page struct {
	Items      []notification.Notification
	NextCursor string
}

var ErrInvalidLimit = fmt.Errorf("%w: limit must be between 1 and %d", notification.ErrInvalid, MaxListLimit)

type Queries struct{ store Store }

func NewQueries(store Store) *Queries { return &Queries{store: store} }

func (q *Queries) List(ctx context.Context, query ListQuery) (Page, error) {
	if err := query.Recipient.validate(); err != nil {
		return Page{}, err
	}
	if query.Limit == 0 {
		query.Limit = DefaultListLimit
	}
	if query.Limit < 1 || query.Limit > MaxListLimit {
		return Page{}, ErrInvalidLimit
	}
	before, err := notification.DecodeCursor(query.Cursor)
	if err != nil {
		return Page{}, err
	}
	// Fetch one extra item to know whether another page exists.
	items, err := q.store.List(ctx, query.Recipient.Audience, query.Recipient.ID, before, query.Limit+1, query.UnreadOnly)
	if err != nil {
		return Page{}, err
	}
	page := Page{Items: items}
	if len(items) > query.Limit {
		page.Items = items[:query.Limit]
		page.NextCursor = notification.CursorOf(page.Items[query.Limit-1]).Encode()
	}
	return page, nil
}

func (q *Queries) CountUnread(ctx context.Context, recipient Recipient) (int64, error) {
	if err := recipient.validate(); err != nil {
		return 0, err
	}
	return q.store.CountUnread(ctx, recipient.Audience, recipient.ID)
}

// ListAfter replays notifications newer than cursor, oldest first.
func (q *Queries) ListAfter(ctx context.Context, recipient Recipient, cursor notification.Cursor, limit int) ([]notification.Notification, error) {
	if err := recipient.validate(); err != nil {
		return nil, err
	}
	return q.store.ListAfter(ctx, recipient.Audience, recipient.ID, cursor, limit)
}

type Commands struct {
	store Store
	now   func() time.Time
}

func NewCommands(store Store) *Commands {
	return &Commands{store: store, now: func() time.Time { return time.Now().UTC() }}
}

// MarkRead returns notification.ErrNotFound for ids the recipient does not own.
func (c *Commands) MarkRead(ctx context.Context, recipient Recipient, id string) error {
	if err := recipient.validate(); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return notification.ErrNotFound
	}
	return c.store.MarkRead(ctx, recipient.Audience, recipient.ID, id, c.now())
}

// MarkAllRead marks everything created at or before `before` (zero = now).
func (c *Commands) MarkAllRead(ctx context.Context, recipient Recipient, before time.Time) (int64, error) {
	if err := recipient.validate(); err != nil {
		return 0, err
	}
	now := c.now()
	if before.IsZero() || before.After(now) {
		before = now
	}
	return c.store.MarkAllRead(ctx, recipient.Audience, recipient.ID, before, now)
}
