//go:build e2e

package mongo

import (
	"context"
	"net/url"
	"os"
	"testing"
	"time"

	notificationapp "github.com/dont-wait/anomaly/internal/application/notification"
	"github.com/dont-wait/anomaly/internal/domain/notification"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func startNotificationMongo(t *testing.T, ctx context.Context) (*mongodrv.Client, *mongodrv.Database) {
	t.Helper()
	container, err := mongodb.Run(ctx, "mongo:7.0", mongodb.WithReplicaSet("rs0"))
	if err != nil {
		t.Fatal(err)
	}
	testcontainers.CleanupContainer(t, container)
	uri, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	q.Set("directConnection", "true")
	parsed.RawQuery = q.Encode()
	client, err := mongodrv.Connect(options.Client().ApplyURI(parsed.String()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	db := client.Database("notification_e2e_" + uuid.NewString()[:8])

	// Apply the real migration so validator and index definitions are tested.
	raw, err := os.ReadFile("../../../migrations/000020_create_notifications.up.json")
	if err != nil {
		t.Fatal(err)
	}
	var commands []bson.D
	if err := bson.UnmarshalExtJSON(raw, false, &commands); err != nil {
		t.Fatal(err)
	}
	for _, command := range commands {
		if err := db.RunCommand(ctx, command).Err(); err != nil {
			t.Fatalf("migration command %v: %v", command[0].Key, err)
		}
	}
	return client, db
}

type notificationTestMapper struct{}

func (notificationTestMapper) Supports(string) bool { return true }

func (notificationTestMapper) Map(_ context.Context, record journal.Record) ([]notificationapp.Request, error) {
	return []notificationapp.Request{{
		Audience: notification.AudienceCustomer, RecipientID: "alice", Type: notification.TypeSystemTest,
		SourceEventID: record.ID, OccurredAt: record.At, Params: map[string]string{"message": "hi"},
	}}, nil
}

func TestNotificationRepositoryWithMongo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	client, db := startNotificationMongo(t, ctx)
	repo := NewNotificationRepository(client, db.Name())
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}

	// TTL index exists.
	specs, err := db.Collection(notificationsCollection).Indexes().ListSpecifications(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ttl := false
	for _, spec := range specs {
		if spec.Name == "idx_notifications_ttl" && spec.ExpireAfterSeconds != nil && *spec.ExpireAfterSeconds == 0 {
			ttl = true
		}
	}
	if !ttl {
		t.Fatal("TTL index missing")
	}

	// Change stream delivers inserts.
	watchCtx, stopWatch := context.WithCancel(ctx)
	received := make(chan notification.Notification, 10)
	go func() {
		_, _ = repo.Watch(watchCtx, nil, nil, func(n notification.Notification) { received <- n })
	}()
	time.Sleep(500 * time.Millisecond) // let the change stream open

	// Dispatching the same record twice yields one notification.
	service := notificationapp.NewService(repo, notificationapp.Config{Retention: time.Hour, MaxEventAge: time.Hour})
	dispatcher := notificationapp.NewDispatcher(service, notificationTestMapper{})
	record := journal.Record{ID: uuid.NewString(), Type: "Anything", At: time.Now().UTC()}
	for i := 0; i < 2; i++ {
		if err := dispatcher.Handle(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	count, err := db.Collection(notificationsCollection).CountDocuments(ctx, bson.M{})
	if err != nil || count != 1 {
		t.Fatalf("expected 1 document, got %d (%v)", count, err)
	}
	select {
	case n := <-received:
		if n.RecipientID != "alice" || n.ReadAt != nil {
			t.Fatalf("unexpected watched notification %+v", n)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("change stream did not deliver insert")
	}
	stopWatch()

	// Inbox, unread count, ownership and read state.
	unread, err := repo.CountUnread(ctx, notification.AudienceCustomer, "alice")
	if err != nil || unread != 1 {
		t.Fatalf("unread=%d err=%v", unread, err)
	}
	items, err := repo.List(ctx, notification.AudienceCustomer, "alice", nil, 10, true)
	if err != nil || len(items) != 1 {
		t.Fatalf("list: %v %v", items, err)
	}
	id := items[0].ID
	if err := repo.MarkRead(ctx, notification.AudienceCustomer, "bob", id, time.Now()); err != notification.ErrNotFound {
		t.Fatalf("foreign mark read: %v", err)
	}
	if err := repo.MarkRead(ctx, notification.AudienceCustomer, "alice", id, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkRead(ctx, notification.AudienceCustomer, "alice", id, time.Now()); err != nil {
		t.Fatalf("mark read must be idempotent: %v", err)
	}
	if unread, _ := repo.CountUnread(ctx, notification.AudienceCustomer, "alice"); unread != 0 {
		t.Fatalf("unread after mark read = %d", unread)
	}
	// Re-inserting must not reset read state.
	if err := dispatcher.Handle(ctx, record); err != nil {
		t.Fatal(err)
	}
	if unread, _ := repo.CountUnread(ctx, notification.AudienceCustomer, "alice"); unread != 0 {
		t.Fatal("duplicate insert reset read state")
	}
	after, err := repo.ListAfter(ctx, notification.AudienceCustomer, "alice", notification.CursorOf(items[0]), 10)
	if err != nil || len(after) != 0 {
		t.Fatalf("list after newest: %v %v", after, err)
	}
}
