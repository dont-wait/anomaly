package mongo

import (
	"context"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/risk"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func riskTestRepository(t *testing.T) (*RiskDecisionRepository, context.Context) {
	t.Helper()
	uri := os.Getenv("MONGO_TEST_URI")
	if uri == "" {
		uri = "mongodb://127.0.0.1:27017/?serverSelectionTimeoutMS=200"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	client, err := mongodrv.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	if err := client.Ping(ctx, nil); err != nil {
		if os.Getenv("MONGO_TEST_URI") != "" {
			t.Fatal(err)
		}
		t.Skipf("MongoDB unavailable: %v", err)
	}
	db := client.Database("risk_test_" + strings.ReplaceAll(uuid.NewString(), "-", ""))
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	repo := NewRiskDecisionRepository(client, db.Name())
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	return repo, ctx
}

func riskTestDecision() risk.Decision {
	return risk.Decision{
		TransferID: uuid.NewString(), SourceEventID: uuid.NewString(), Decision: risk.DecisionAllow,
		Assessment: risk.Assessment{RiskScore: 0.08, ModelVersion: "v1", EvaluatedAt: time.Now().UTC()},
	}
}

func TestRiskDecisionFindExcludesExpiredBeforeTTLDeletion(t *testing.T) {
	repo, ctx := riskTestRepository(t)
	d := riskTestDecision()
	d.ExpiresAt = time.Now().UTC().Add(-time.Second)
	// No TTL index during this assertion: expired data is deliberately still present.
	if err := repo.collection.Indexes().DropOne(ctx, "ttl_risk_decision"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	if _, found, err := repo.Find(ctx, d.TransferID); err != nil || found {
		t.Fatalf("expired decision returned: found=%v err=%v", found, err)
	}
	if n, err := repo.collection.CountDocuments(ctx, bson.M{"transfer_id": d.TransferID}); err != nil || n != 1 {
		t.Fatalf("expired fixture missing: %d/%v", n, err)
	}
}

func TestRiskDecisionRedeliveryCannotOverwriteTerminalOrFinalized(t *testing.T) {
	repo, ctx := riskTestRepository(t)
	d := riskTestDecision()
	pending := d
	pending.Decision = risk.DecisionPending
	if err := repo.Save(ctx, pending); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	original, found, err := repo.Find(ctx, d.TransferID)
	if err != nil || !found || original.Decision != risk.DecisionAllow {
		t.Fatalf("pending did not advance: %+v/%v", original, err)
	}
	blocked := d
	blocked.Decision, blocked.Assessment.IsAnomalous = risk.DecisionBlock, true
	for _, replay := range []risk.Decision{pending, blocked, d} {
		if err := repo.Save(ctx, replay); err != nil {
			t.Fatal(err)
		}
	}
	current, _, _ := repo.Find(ctx, d.TransferID)
	if !reflect.DeepEqual(current, original) {
		t.Fatal("terminal record changed on redelivery")
	}
	if err := repo.Finalize(ctx, d.TransferID); err != nil {
		t.Fatal(err)
	}
	var finalized risk.Decision
	if err := repo.collection.FindOne(ctx, bson.M{"transfer_id": d.TransferID}).Decode(&finalized); err != nil {
		t.Fatal(err)
	}
	for _, replay := range []risk.Decision{pending, blocked, d} {
		if err := repo.Save(ctx, replay); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.Finalize(ctx, d.TransferID); err != nil {
		t.Fatal(err)
	}
	var after risk.Decision
	if err := repo.collection.FindOne(ctx, bson.M{"transfer_id": d.TransferID}).Decode(&after); err != nil {
		t.Fatal(err)
	}
	if !after.Finalized || !reflect.DeepEqual(after, finalized) {
		t.Fatal("finalized audit changed on replay")
	}
}

func TestRiskDecisionConcurrentRedeliveryAndQuarantine(t *testing.T) {
	repo, ctx := riskTestRepository(t)
	d := riskTestDecision()
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			if err := repo.Save(ctx, d); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if n, err := repo.collection.CountDocuments(ctx, bson.M{}); err != nil || n != 1 {
		t.Fatalf("duplicate records: %d/%v", n, err)
	}
	for range 2 {
		if err := repo.Reject(ctx, "risk", 1, 42, []byte(`{`), "invalid JSON"); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := repo.rejected.CountDocuments(ctx, bson.M{}); err != nil || n != 1 {
		t.Fatalf("duplicate rejections: %d/%v", n, err)
	}
}
