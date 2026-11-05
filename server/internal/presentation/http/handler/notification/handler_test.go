package notification

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/application/account/queries"
	notificationapp "github.com/dont-wait/anomaly/internal/application/notification"
	"github.com/dont-wait/anomaly/internal/domain/auth"
	notificationdomain "github.com/dont-wait/anomaly/internal/domain/notification"
	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
	"github.com/rs/zerolog"
)

type tokenStub struct{ queries.TokenService }

// The bearer token is the user id, so tests can act as different users.
func (tokenStub) Parse(token string) (*auth.Claims, error) {
	return &auth.Claims{UserID: token, Role: "user", ExpiresAt: time.Now().Add(time.Hour)}, nil
}

type fakeStore struct {
	mu    sync.Mutex
	items []notificationdomain.Notification
}

func (s *fakeStore) InsertMany(_ context.Context, items []notificationdomain.Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append(s.items, items...)
	return nil
}

func (s *fakeStore) owned(audience notificationdomain.Audience, recipient string) []notificationdomain.Notification {
	var out []notificationdomain.Notification
	for _, item := range s.items {
		if item.Audience == audience && item.RecipientID == recipient {
			out = append(out, item)
		}
	}
	return out
}

func (s *fakeStore) List(_ context.Context, a notificationdomain.Audience, r string, _ *notificationdomain.Cursor, limit int, _ bool) ([]notificationdomain.Notification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.owned(a, r)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *fakeStore) ListAfter(_ context.Context, a notificationdomain.Audience, r string, after notificationdomain.Cursor, limit int) ([]notificationdomain.Notification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []notificationdomain.Notification
	for _, item := range s.owned(a, r) {
		if isAfter(item, after) && len(out) < limit {
			out = append(out, item)
		}
	}
	return out, nil
}

func (s *fakeStore) CountUnread(_ context.Context, a notificationdomain.Audience, r string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int64
	for _, item := range s.owned(a, r) {
		if item.ReadAt == nil {
			n++
		}
	}
	return n, nil
}

func (s *fakeStore) MarkRead(_ context.Context, a notificationdomain.Audience, r, id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, item := range s.items {
		if item.ID == id && item.Audience == a && item.RecipientID == r {
			s.items[i].ReadAt = &at
			return nil
		}
	}
	return notificationdomain.ErrNotFound
}

func (s *fakeStore) MarkAllRead(context.Context, notificationdomain.Audience, string, time.Time, time.Time) (int64, error) {
	return 0, nil
}

func item(id, recipient string, createdAt time.Time) notificationdomain.Notification {
	return notificationdomain.Notification{
		ID: id, Audience: notificationdomain.AudienceCustomer, RecipientID: recipient, Type: notificationdomain.TypeSystemTest,
		Category: notificationdomain.CategorySystem, Severity: notificationdomain.SeverityInfo, Title: "t", Body: "b",
		SourceEventID: "s", OccurredAt: createdAt, CreatedAt: createdAt, ExpiresAt: createdAt.Add(time.Hour),
	}
}

func newTestServer(store *fakeStore, hub *notificationapp.Hub) *http.ServeMux {
	mux := http.NewServeMux()
	handler := NewHandler(notificationapp.NewQueries(store), notificationapp.NewCommands(store), hub, zerolog.Nop())
	RegisterRoutes(openapi.NewRegistry(mux, false), handler, tokenStub{})
	return mux
}

func do(mux http.Handler, method, path, user string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if user != "" {
		req.Header.Set("Authorization", "Bearer "+user)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestRoutesRequireJWT(t *testing.T) {
	mux := newTestServer(&fakeStore{}, notificationapp.NewHub(5))
	for _, route := range []struct{ method, path string }{
		{"GET", "/api/notifications"},
		{"GET", "/api/notifications/unread-count"},
		{"PATCH", "/api/notifications/x/read"},
		{"POST", "/api/notifications/read-all"},
		{"GET", "/api/notifications/stream"},
	} {
		if rec := do(mux, route.method, route.path, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: got %d", route.method, route.path, rec.Code)
		}
	}
}

func TestListValidatesLimitAndScopesToOwner(t *testing.T) {
	now := time.Now().UTC()
	store := &fakeStore{items: []notificationdomain.Notification{item("a1", "alice", now), item("b1", "bob", now)}}
	mux := newTestServer(store, notificationapp.NewHub(5))
	for _, limit := range []string{"0", "51", "abc"} {
		if rec := do(mux, "GET", "/api/notifications?limit="+limit, "alice"); rec.Code != http.StatusBadRequest {
			t.Errorf("limit=%s: got %d", limit, rec.Code)
		}
	}
	rec := do(mux, "GET", "/api/notifications?limit=20", "alice")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	var body struct {
		Data ListResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Data.Items) != 1 || body.Data.Items[0].ID != "a1" {
		t.Fatalf("unexpected list: %s", rec.Body)
	}
}

func TestMarkReadOtherUsersNotificationIsNotFound(t *testing.T) {
	store := &fakeStore{items: []notificationdomain.Notification{item("a1", "alice", time.Now().UTC())}}
	mux := newTestServer(store, notificationapp.NewHub(5))
	if rec := do(mux, "PATCH", "/api/notifications/a1/read", "bob"); rec.Code != http.StatusNotFound {
		t.Fatalf("bob marking alice's notification: got %d", rec.Code)
	}
	if rec := do(mux, "PATCH", "/api/notifications/a1/read", "alice"); rec.Code != http.StatusOK {
		t.Fatalf("owner mark read: got %d", rec.Code)
	}
	if store.items[0].ReadAt == nil {
		t.Fatal("notification not marked read")
	}
}

func TestStreamReplaysThenPushesLive(t *testing.T) {
	base := time.Date(2026, 11, 4, 8, 0, 0, 0, time.UTC)
	old, replayed := item("n1", "alice", base), item("n2", "alice", base.Add(time.Second))
	store := &fakeStore{items: []notificationdomain.Notification{old, replayed}}
	hub := notificationapp.NewHub(5)
	mux := newTestServer(store, hub)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/api/notifications/stream", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer alice")
	req.Header.Set("Last-Event-ID", notificationdomain.CursorOf(old).Encode())
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { mux.ServeHTTP(rec, req); close(done) }()

	live := item("n3", "alice", base.Add(2*time.Second))
	key := notificationdomain.RecipientKey(notificationdomain.AudienceCustomer, "alice")
	for deadline := time.Now().Add(2 * time.Second); hub.Subscribers(key) == 0; {
		if time.Now().After(deadline) {
			t.Fatal("stream never subscribed")
		}
		time.Sleep(5 * time.Millisecond)
	}
	store.mu.Lock()
	store.items = append(store.items, live)
	store.mu.Unlock()
	hub.Publish(replayed) // duplicate of the replayed item: must be skipped
	hub.Publish(item("x", "bob", base.Add(3*time.Second)))
	hub.Publish(live)
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	out := rec.Body.String()
	if rec.Header().Get("Content-Type") != "text/event-stream" || rec.Header().Get("X-Accel-Buffering") != "no" {
		t.Fatalf("missing SSE headers: %v", rec.Header())
	}
	if strings.Count(out, "event: unread-count") < 2 {
		t.Errorf("expected unread-count on open and after live item:\n%s", out)
	}
	if strings.Contains(out, `"id":"n1"`) || strings.Contains(out, `"id":"x"`) {
		t.Errorf("stream leaked already-seen or foreign notification:\n%s", out)
	}
	if strings.Count(out, `"id":"n2"`) != 1 || strings.Count(out, `"id":"n3"`) != 1 {
		t.Errorf("expected n2 replayed once and n3 live once:\n%s", out)
	}
	if !strings.Contains(out, "id: "+notificationdomain.CursorOf(live).Encode()+"\nevent: notification\n") {
		t.Errorf("event id must be the cursor:\n%s", out)
	}
}

func TestStreamRejectsTooManyConnections(t *testing.T) {
	hub := notificationapp.NewHub(1)
	_, cancel, _ := hub.Subscribe(notificationdomain.RecipientKey(notificationdomain.AudienceCustomer, "alice"))
	defer cancel()
	if rec := do(newTestServer(&fakeStore{}, hub), "GET", "/api/notifications/stream", "alice"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestStreamWithoutCursorSendsResumeCursor(t *testing.T) {
	newest := item("n1", "alice", time.Date(2026, 11, 4, 8, 0, 0, 0, time.UTC))
	store := &fakeStore{items: []notificationdomain.Notification{newest}}
	hub := notificationapp.NewHub(5)
	mux := newTestServer(store, hub)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/api/notifications/stream", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer alice")
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { mux.ServeHTTP(rec, req); close(done) }()
	key := notificationdomain.RecipientKey(notificationdomain.AudienceCustomer, "alice")
	for deadline := time.Now().Add(2 * time.Second); hub.Subscribers(key) == 0; {
		if time.Now().After(deadline) {
			t.Fatal("stream never subscribed")
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	out := rec.Body.String()
	want := "id: " + notificationdomain.CursorOf(newest).Encode() + "\nevent: cursor\n"
	if !strings.Contains(out, want) {
		t.Fatalf("missing resume cursor:\n%s", out)
	}
	if strings.Contains(out, `"id":"n1"`) {
		t.Fatalf("existing notifications must not be replayed without a cursor:\n%s", out)
	}
}
