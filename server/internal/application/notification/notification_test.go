package notification

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/notification"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
)

type memoryStore struct {
	Store
	inserted []notification.Notification
	listed   struct {
		before *notification.Cursor
		limit  int
	}
	items []notification.Notification
}

func (m *memoryStore) InsertMany(_ context.Context, items []notification.Notification) error {
	m.inserted = append(m.inserted, items...)
	return nil
}

func (m *memoryStore) List(_ context.Context, _ notification.Audience, _ string, before *notification.Cursor, limit int, _ bool) ([]notification.Notification, error) {
	m.listed.before, m.listed.limit = before, limit
	if limit > len(m.items) {
		return m.items, nil
	}
	return m.items[:limit], nil
}

func newTestService(store Store, now time.Time) *Service {
	s := NewService(store, Config{Retention: 24 * time.Hour, MaxEventAge: time.Hour})
	s.now = func() time.Time { return now }
	return s
}

func testRequest(at time.Time) Request {
	return Request{
		Audience: notification.AudienceCustomer, RecipientID: "user-1", Type: notification.TypeSystemTest,
		SourceEventID: "evt-1", OccurredAt: at, Params: map[string]string{"message": "hello"},
	}
}

func TestNotifyBuildsDeterministicNotification(t *testing.T) {
	now := time.Date(2026, 11, 4, 8, 0, 0, 999999, time.UTC)
	store := &memoryStore{}
	if err := newTestService(store, now).Notify(context.Background(), testRequest(now.Add(-time.Minute))); err != nil {
		t.Fatal(err)
	}
	if len(store.inserted) != 1 {
		t.Fatalf("expected 1 insert, got %d", len(store.inserted))
	}
	got := store.inserted[0]
	if got.ID != notification.NotificationID("evt-1", notification.TypeSystemTest, "user-1") ||
		got.Category != notification.CategorySystem || got.Body != "hello" ||
		!got.ExpiresAt.Equal(now.Add(-time.Minute).Add(24*time.Hour)) || got.CreatedAt.Nanosecond()%int(time.Millisecond) != 0 {
		t.Fatalf("unexpected notification: %+v", got)
	}
}

func TestNotifySkipsEventsOlderThanMaxAge(t *testing.T) {
	now := time.Now().UTC()
	store := &memoryStore{}
	if err := newTestService(store, now).Notify(context.Background(), testRequest(now.Add(-2*time.Hour))); err != nil {
		t.Fatal(err)
	}
	if len(store.inserted) != 0 {
		t.Fatal("old event must be skipped")
	}
}

func TestNotifyRejectsMissingParamAndUnknownType(t *testing.T) {
	now := time.Now().UTC()
	service := newTestService(&memoryStore{}, now)
	missing := testRequest(now)
	missing.Params = nil
	if err := service.Notify(context.Background(), missing); !errors.Is(err, ErrMissingParam) || !errors.Is(err, notification.ErrInvalid) {
		t.Fatalf("expected missing param error, got %v", err)
	}
	unknown := testRequest(now)
	unknown.Type = "NOPE"
	if err := service.Notify(context.Background(), unknown); !errors.Is(err, ErrUnknownTemplate) {
		t.Fatalf("expected unknown template error, got %v", err)
	}
}

func TestRegisterPanicsOnDuplicate(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	tmpl, _ := Lookup(notification.TypeSystemTest)
	Register(tmpl)
}

func TestListPagesWithNextCursor(t *testing.T) {
	base := time.Date(2026, 11, 4, 8, 0, 0, 0, time.UTC)
	store := &memoryStore{}
	for i := 0; i < 3; i++ {
		store.items = append(store.items, notification.Notification{ID: string(rune('c' - i)), CreatedAt: base.Add(-time.Duration(i) * time.Second)})
	}
	recipient := Recipient{Audience: notification.AudienceCustomer, ID: "user-1"}
	page, err := NewQueries(store).List(context.Background(), ListQuery{Recipient: recipient, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.NextCursor != notification.CursorOf(store.items[1]).Encode() || store.listed.limit != 3 {
		t.Fatalf("unexpected page: %+v (limit %d)", page, store.listed.limit)
	}
	store.items = store.items[:1]
	page, _ = NewQueries(store).List(context.Background(), ListQuery{Recipient: recipient, Limit: 2})
	if page.NextCursor != "" {
		t.Fatal("last page must have empty cursor")
	}
	for _, limit := range []int{-1, MaxListLimit + 1} {
		if _, err := NewQueries(store).List(context.Background(), ListQuery{Recipient: recipient, Limit: limit}); !errors.Is(err, ErrInvalidLimit) {
			t.Errorf("limit %d: expected ErrInvalidLimit, got %v", limit, err)
		}
	}
}

func TestHubDeliversOnlyToRecipient(t *testing.T) {
	hub := NewHub(5)
	mine, cancelMine, _ := hub.Subscribe(notification.RecipientKey(notification.AudienceCustomer, "a"))
	defer cancelMine()
	other, cancelOther, _ := hub.Subscribe(notification.RecipientKey(notification.AudienceCustomer, "b"))
	defer cancelOther()
	hub.Publish(notification.Notification{ID: "1", Audience: notification.AudienceCustomer, RecipientID: "a"})
	if got := <-mine; got.ID != "1" {
		t.Fatalf("unexpected %+v", got)
	}
	select {
	case n := <-other:
		t.Fatalf("other recipient received %+v", n)
	default:
	}
}

func TestHubClosesSlowClient(t *testing.T) {
	hub := NewHub(5)
	key := notification.RecipientKey(notification.AudienceCustomer, "a")
	updates, cancel, _ := hub.Subscribe(key)
	defer cancel()
	for i := 0; i < hubClientBuffer+1; i++ {
		hub.Publish(notification.Notification{Audience: notification.AudienceCustomer, RecipientID: "a"})
	}
	for i := 0; i < hubClientBuffer; i++ {
		<-updates
	}
	if _, ok := <-updates; ok {
		t.Fatal("slow client must be closed")
	}
}

func TestHubLimitsConnectionsPerRecipient(t *testing.T) {
	hub := NewHub(2)
	key := notification.RecipientKey(notification.AudienceCustomer, "a")
	_, c1, _ := hub.Subscribe(key)
	_, c2, _ := hub.Subscribe(key)
	if _, _, err := hub.Subscribe(key); !errors.Is(err, ErrTooManyConnections) {
		t.Fatalf("expected limit error, got %v", err)
	}
	c1()
	c1() // idempotent
	if _, c3, err := hub.Subscribe(key); err != nil {
		t.Fatalf("slot must be released: %v", err)
	} else {
		c3()
	}
	c2()
}

type fakeMapper struct{ calls int }

func (f *fakeMapper) Supports(eventType string) bool { return eventType == "Wanted" }
func (f *fakeMapper) Map(_ context.Context, record journal.Record) ([]Request, error) {
	f.calls++
	return []Request{testRequest(record.At)}, nil
}

func TestDispatcherOnlyCallsSupportingMappers(t *testing.T) {
	store := &memoryStore{}
	mapper := &fakeMapper{}
	dispatcher := NewDispatcher(newTestService(store, time.Now().UTC()), mapper)
	if err := dispatcher.Handle(context.Background(), journal.Record{ID: "1", Type: "Other", At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Handle(context.Background(), journal.Record{ID: "2", Type: "Wanted", At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if mapper.calls != 1 || len(store.inserted) != 1 {
		t.Fatalf("calls=%d inserted=%d", mapper.calls, len(store.inserted))
	}
}

func TestHubDisconnectAllKeepsAcceptingSubscribers(t *testing.T) {
	hub := NewHub(5)
	key := notification.RecipientKey(notification.AudienceCustomer, "a")
	updates, cancel, _ := hub.Subscribe(key)
	defer cancel()
	hub.DisconnectAll()
	if _, ok := <-updates; ok {
		t.Fatal("existing stream must be closed so the client replays")
	}
	again, cancelAgain, err := hub.Subscribe(key)
	if err != nil {
		t.Fatalf("reconnect must be accepted: %v", err)
	}
	defer cancelAgain()
	hub.Publish(notification.Notification{ID: "1", Audience: notification.AudienceCustomer, RecipientID: "a"})
	if got := <-again; got.ID != "1" {
		t.Fatalf("unexpected %+v", got)
	}
}
