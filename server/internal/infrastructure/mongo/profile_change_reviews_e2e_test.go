//go:build e2e

package mongo

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestProfileReviewTransactionsHistoryAndRace(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	container, err := mongodb.Run(ctx, "mongo:7.0", mongodb.WithReplicaSet("rs0"))
	if err != nil {
		t.Fatal(err)
	}
	testcontainers.CleanupContainer(t, container)
	uri, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	parsedURI, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	q := parsedURI.Query()
	q.Set("directConnection", "true")
	parsedURI.RawQuery = q.Encode()
	client, err := mongodrv.Connect(options.Client().ApplyURI(parsedURI.String()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	repo := NewAccountAggregateRepository(client, "profile_review_"+uuid.NewString()[:8])
	// Test the actual migration (including collection validator, index creation,
	// and backfill command) rather than silently creating a blank collection.
	for _, migrationFile := range []string{
		"../../../migrations/000023_profile_change_reviews.up.json",
		"../../../migrations/000024_profile_change_search.up.json",
	} {
		migration, readErr := os.ReadFile(migrationFile)
		if readErr != nil {
			t.Fatal(readErr)
		}
		var commands []bson.D
		if err := bson.UnmarshalExtJSON(migration, false, &commands); err != nil {
			t.Fatal(err)
		}
		for _, command := range commands {
			if err := repo.profileChangeReviews.Database().RunCommand(ctx, command).Err(); err != nil {
				t.Fatalf("migration %s command %s: %v", migrationFile, command[0].Key, err)
			}
		}
		if strings.Contains(migrationFile, "000023_") {
			// Existing records must receive indexed keys from migration 24.
			seed := profileChangeReviewRecord{
				ID: "migration-seed", AccountID: "seed-account", CustomerID: bson.NewObjectID(),
				Username: "seed", SearchText: "seed-account seed Tên cũ Tên mới",
				Status: "PENDING", RequestedAt: time.Now().UTC(),
				Proposed: profileChangeRecord{ID: "migration-seed", FullName: "Tên mới", Status: "PENDING"},
			}
			if _, err := repo.profileChangeReviews.InsertOne(ctx, seed); err != nil {
				t.Fatal(err)
			}
		}
	}
	var backfilled profileChangeReviewRecord
	if err := repo.profileChangeReviews.FindOne(ctx, bson.M{"_id": "migration-seed"}).Decode(&backfilled); err != nil {
		t.Fatal(err)
	}
	if len(backfilled.SearchKeys) == 0 {
		t.Fatal("migration 24 failed to backfill search keys")
	}
	if _, err := repo.profileChangeReviews.DeleteOne(ctx, bson.M{"_id": "migration-seed"}); err != nil {
		t.Fatal(err)
	}
	if err = repo.VerifyProfileChangeMigration(ctx); err != nil {
		t.Fatal(err)
	}
	customerID, accountID := bson.NewObjectID(), bson.NewObjectID()
	if _, err = repo.customers.col.InsertOne(ctx, customerRecord{Id: customerID, Profile: customerProfileRecord{FullName: "Tên ban đầu", Email: "test@example.com"}}); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.accounts.col.InsertOne(ctx, bson.M{"_id": accountID, "version": 1}); err != nil {
		t.Fatal(err)
	}
	account := &accountdomain.UserAccount{Id: accountID.Hex(), CustomerId: customerID.Hex(), Username: "test"}
	t1 := time.Now().UTC().Truncate(time.Millisecond)
	first := &accountdomain.ProfileChangeRequest{Id: uuid.NewString(), FullName: "Tên thứ nhất", Status: "PENDING", RequestedAt: t1}
	if err = repo.SubmitProfileChange(ctx, account, first); err != nil {
		t.Fatal(err)
	}
	if err = repo.DecideProfileChange(ctx, "admin", account.Id, first.Id, "APPROVED", "", t1.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	var customer customerRecord
	if err = repo.customers.col.FindOne(ctx, bson.M{"_id": customerID}).Decode(&customer); err != nil {
		t.Fatal(err)
	}
	if customer.Profile.FullName != "Tên thứ nhất" {
		t.Fatalf("approval did not update profile: %s", customer.Profile.FullName)
	}
	approved, err := repo.ListProfileChanges(ctx, accountdomain.ProfileChangeListOptions{
		Page: 1, PageSize: 10, Status: "APPROVED", Search: "thứ nhất",
	})
	if err != nil {
		t.Fatal(err)
	}
	if approved.Total != 1 || len(approved.Items) != 1 || approved.Items[0].Email != "test@example.com" {
		t.Fatalf("approved prefix search or identity: %#v", approved)
	}
	stored := profileChangeReviewRecord{}
	if err := repo.profileChangeReviews.FindOne(ctx, bson.M{"_id": first.Id}).Decode(&stored); err != nil {
		t.Fatal(err)
	}
	if stored.SearchText == "" || len(stored.SearchKeys) == 0 {
		t.Fatal("search index was not updated")
	}
	second := &accountdomain.ProfileChangeRequest{Id: uuid.NewString(), FullName: "Tên thứ hai", Status: "PENDING", RequestedAt: t1.Add(2 * time.Second)}
	if err = repo.SubmitProfileChange(ctx, account, second); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	responses := make(chan error, 2)
	for _, decision := range []string{"APPROVED", "REJECTED"} {
		wg.Add(1)
		go func(decision string) {
			defer wg.Done()
			reason := ""
			if decision == "REJECTED" {
				reason = "Lý do"
			}
			responses <- repo.DecideProfileChange(ctx, "admin2", account.Id, second.Id, decision, reason, t1.Add(3*time.Second))
		}(decision)
	}
	wg.Wait()
	close(responses)
	ok, closed := 0, 0
	for e := range responses {
		if e == nil {
			ok++
		} else if errors.Is(e, accountdomain.ErrProfileChangeClosed) {
			closed++
		} else {
			t.Errorf("unexpected decision error: %v", e)
		}
	}
	if ok != 1 || closed != 1 {
		t.Fatalf("expected exactly one successful decision: %d ok %d conflict", ok, closed)
	}
	list, err := repo.ListProfileChanges(ctx, accountdomain.ProfileChangeListOptions{Page: 1, PageSize: 10, Status: "ALL"})
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 2 || len(list.Items) != 2 || list.Items[1].Previous.FullName != "Tên ban đầu" {
		t.Fatalf("history: %#v", list)
	}
	one, err := repo.ListProfileChanges(ctx, accountdomain.ProfileChangeListOptions{Page: 1, PageSize: 1, Status: "ALL"})
	if err != nil || len(one.Items) != 1 || one.NextCursor == "" {
		t.Fatalf("cursor first page: %#v %v", one, err)
	}
	third := &accountdomain.ProfileChangeRequest{Id: uuid.NewString(), FullName: "Tên thứ ba", Status: "PENDING", RequestedAt: t1.Add(4 * time.Second)}
	if err := repo.SubmitProfileChange(ctx, account, third); err != nil {
		t.Fatal(err)
	}
	next, err := repo.ListProfileChanges(ctx, accountdomain.ProfileChangeListOptions{Page: 2, PageSize: 1, Status: "ALL", Cursor: one.NextCursor})
	if err != nil || len(next.Items) != 1 || next.Items[0].Change.Id == one.Items[0].Change.Id || next.Items[0].Change.Id == third.Id {
		t.Fatalf("cursor repeated/skipped after new request: %#v %v", next, err)
	}
}
