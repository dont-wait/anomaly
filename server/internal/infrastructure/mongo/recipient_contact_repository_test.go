package mongo

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestRecipientContactRepositoryListScopesOwners(t *testing.T) {
	uri := os.Getenv("MONGO_TEST_URI")
	if uri == "" {
		uri = os.Getenv("MONGO_URI")
	}
	if uri == "" {
		uri = "mongodb://127.0.0.1:27017/?serverSelectionTimeoutMS=200"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, err := mongodrv.Connect((&options.ClientOptions{}).ApplyURI(uri))
	if err != nil {
		t.Skipf("MongoDB is unavailable: %v", err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	if err := client.Ping(ctx, nil); err != nil {
		t.Skipf("MongoDB is unavailable: %v", err)
	}

	dbName := "anomaly_contact_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	db := client.Database(dbName)
	defer func() { _ = db.Drop(context.Background()) }()
	collection := db.Collection("recipient_contacts")

	ownerA := bson.NewObjectID()
	ownerB := bson.NewObjectID()
	_, err = collection.InsertMany(ctx, []any{
		bson.M{
			"owner_account_id": ownerA, "recipient_account_no": "99999180147",
			"recipient_name": "Owner A contact", "bank_code": "ANOMALY",
			"created_at": time.Now().UTC(), "updated_at": time.Now().UTC(),
		},
		bson.M{
			"owner_account_id": ownerB, "recipient_account_no": "99999180148",
			"recipient_name": "Owner B contact", "bank_code": "ANOMALY",
			"created_at": time.Now().UTC(), "updated_at": time.Now().UTC(),
		},
	})
	if err != nil {
		t.Fatalf("insert contacts: %v", err)
	}

	repo := NewRecipientContactRepository(client, dbName)
	contacts, err := repo.List(ctx, ownerA, "", 50)
	if err != nil {
		t.Fatalf("list contacts: %v", err)
	}
	if len(contacts) != 1 {
		t.Fatalf("owner A received %d contacts, want 1", len(contacts))
	}
	if contacts[0].OwnerAccountID != ownerA || contacts[0].RecipientAccountNo != "99999180147" {
		t.Fatalf("owner A received contact %#v from another owner", contacts[0])
	}
}
