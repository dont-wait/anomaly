//go:build e2e

package mongo

import (
	"context"
	"fmt"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/risk"
	"github.com/dont-wait/anomaly/internal/domain/riskassessment"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/dont-wait/anomaly/internal/timeutil"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestTransactionReportMongoE2E(t *testing.T) {
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
	client, err := mongodrv.Connect(options.Client().ApplyURI(uri).SetDirect(true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	now := time.Now().In(timeutil.VietnamLocation)
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, timeutil.VietnamLocation).AddDate(0, 0, -2).UTC()
	to := from.AddDate(0, 0, 1)
	newRepository := func(t *testing.T) *RiskAssessmentRepository {
		t.Helper()
		db := client.Database("report_v2_" + uuid.NewString()[:8])
		t.Cleanup(func() { _ = db.Drop(context.Background()) })
		r := NewRiskAssessmentRepository(client, db.Name())
		if err := r.EnsureIndexes(ctx); err != nil {
			t.Fatal(err)
		}
		_, err := r.transactions.Indexes().CreateOne(ctx, mongodrv.IndexModel{
			Keys: bson.D{{Key: "transaction_id", Value: 1}}, Options: options.Index().SetUnique(true),
		})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	transfer := func(status string, created time.Time, amount int64) TransferDocument {
		tx := TransferDocument{
			ID: uuid.NewString(), Type: "transfer", Status: status, CreatedAt: created,
			Amount: amount, Fee: amount / 100, Currency: "VND",
		}
		tx.Source.AccountNo, tx.Source.Name = "11111111111111", "Source"
		tx.Destination.AccountNo, tx.Destination.Name = "22222222222222", "Destination"
		return tx
	}
	assessment := func(tx TransferDocument, score float64, version string) riskassessment.Assessment {
		label, decision := 0, risk.DecisionAllow
		if score >= 0.5 {
			label = 1
		}
		if score > 0.7 {
			decision = risk.DecisionBlock
		}
		return riskassessment.Assessment{
			ID: uuid.NewString(), SchemaVersion: 1, TransactionID: tx.ID, EventID: journal.EventID(tx.ID, journal.Created),
			OccurredAt: tx.CreatedAt, ScoredAt: tx.CreatedAt.Add(time.Second), ReceivedAt: tx.CreatedAt.Add(2 * time.Second),
			RiskScore: score, Threshold: 0.5, PredictedLabel: label, Decision: decision,
			ModelVersion: version, FeatureVersion: "features-v1", Finalized: true, Handled: true,
		}
	}
	insert := func(t *testing.T, r *RiskAssessmentRepository, tx TransferDocument, assessments ...riskassessment.Assessment) {
		t.Helper()
		if _, err := r.transactions.InsertOne(ctx, tx); err != nil {
			t.Fatal(err)
		}
		for _, a := range assessments {
			if _, err := r.collection.InsertOne(ctx, a); err != nil {
				t.Fatal(err)
			}
		}
	}

	t.Run("contract cohorts", func(t *testing.T) {
		r := newRepository(t)
		txs := []TransferDocument{
			transfer("success", to.Add(-time.Minute), 100000),
			transfer("success", from.Add(12*time.Hour), 200000),
			transfer("failed", from.Add(13*time.Hour), 300000),
			transfer("cancelled", from.Add(14*time.Hour), 400000),
			transfer("awaiting_otp", to.Add(-30*time.Second), 500000),
			transfer("awaiting_otp", from.Add(20*time.Hour), 600000),
			transfer("success", from.Add(-time.Minute), 700000),
		}
		completed := []time.Time{to.Add(time.Minute), from.Add(12*time.Hour + time.Minute), from.Add(time.Minute)}
		txs[0].CompletedAt, txs[1].CompletedAt, txs[6].CompletedAt = &completed[0], &completed[1], &completed[2]
		txs[4].OTPExpiresAt, txs[5].OTPExpiresAt = time.Now().Add(time.Hour), from.Add(20*time.Hour+5*time.Minute)
		txs[2].FailureReason = "anomalous_transfer"
		for i, score := range map[int]float64{0: 0.2, 1: 0.1, 2: 0.9, 6: 0.1} {
			a := assessment(txs[i], score, "applied-model")
			if i == 0 {
				a.ScoredAt, a.ReceivedAt = to.Add(10*time.Second), to.Add(30*time.Second)
			}
			txs[i].RiskAssessment = &risk.Assessment{
				Decision: a.Decision, RiskScore: score,
				IsAnomalous: a.Decision == risk.DecisionBlock, ModelVersion: a.ModelVersion, EvaluatedAt: a.ScoredAt,
			}
		}
		for i, tx := range txs {
			if i == 4 {
				insert(t, r, tx, assessment(tx, 0, "observed-zero"))
			} else {
				insert(t, r, tx)
			}
		}
		// Neither feed-like/non-transfer rows nor intents at the upper boundary
		// contribute to the report's created cohort.
		nonTransfer := transfer("success", from, 999999)
		nonTransfer.Type = "loan_disbursement"
		insert(t, r, nonTransfer)
		insert(t, r, transfer("cancelled", to, 999999))
		report, err := r.TransactionReport(ctx, from, to)
		if err != nil {
			t.Fatal(err)
		}
		c, m, scores := report.Summary.Created, report.Summary.Completed, report.Summary.Risk
		if c.TransactionCount != 6 || c.SuccessCount != 2 || c.FailedCount != 1 || c.CancelledCount != 1 || c.ExpiredCount != 1 || c.PendingCount != 1 {
			t.Fatalf("created = %+v", c)
		}
		if m.Count != 2 || m.TotalAmount != 900000 || m.TotalFee != 9000 || *m.AverageAmount != 450000 ||
			scores.AssessedCount != 4 || scores.AllowCount != 3 || scores.ModelBlockCount != 1 ||
			scores.ActualBlockedCount != 1 || math.Abs(*scores.AverageScore-0.3) > 1e-12 || len(report.DataQuality) != 0 {
			t.Fatalf("completed/risk/quality: %+v / %+v / %+v", m, scores, report.DataQuality)
		}
		if report.Previous.Created.TransactionCount != 1 || report.Previous.Completed.Count != 0 ||
			len(report.Transactions) != 6 || report.Hourly.Completed[0].TotalAmount != 700000 {
			t.Fatalf("cohorts = %+v", report)
		}
	})

	t.Run("all assessments across batches and finalized handled records", func(t *testing.T) {
		r := newRepository(t)
		// Cross both the old 50-row monitor limit and the 500-ID query batch.
		for i := range 501 {
			tx := transfer("awaiting_otp", from.Add(time.Hour), 10000)
			tx.Source.AccountNo = fmt.Sprintf("%014d", i)
			tx.OTPExpiresAt = time.Now().Add(time.Hour)
			old, latest := assessment(tx, 0.1, "old"), assessment(tx, 0.9, "latest")
			latest.ScoredAt = latest.ScoredAt.Add(time.Minute)
			latest.ReceivedAt = latest.ReceivedAt.Add(time.Minute)
			future := assessment(tx, 0.1, "future")
			future.ReceivedAt = time.Now().Add(time.Hour)
			insert(t, r, tx, old, latest, future)
		}
		report, err := r.TransactionReport(ctx, from, to)
		if err != nil {
			t.Fatal(err)
		}
		if len(report.Transactions) != 501 || report.Summary.Risk.AssessedCount != 501 || report.Summary.Risk.ModelBlockCount != 501 ||
			report.Summary.Risk.ActualBlockedCount != 0 || report.Summary.Created.PendingCount != 501 {
			t.Fatalf("batch/count/status error: %+v", report.Summary)
		}
		if report.TopRiskSources[0].AccountNo != "00000000000000" || report.TopRiskReceivers[0].Count != 501 || len(report.DataQuality) != 0 {
			t.Fatalf("accounts/quality: %+v / %+v", report.TopRiskSources, report.DataQuality)
		}
	})

	t.Run("completion fallback and global anomalies outside created window", func(t *testing.T) {
		r := newRepository(t)
		old := from.AddDate(0, -1, 0)
		for i := range 4 {
			tx := transfer("success", old, 10000)
			switch i {
			case 0:
				tx.PostedAt = from
			case 1:
				tx.PostedAt = from
				tx.CompletedAt = &time.Time{} // explicit Go-zero also falls back
			case 2: // both missing; cannot be assigned to any report date
			case 3:
				invalid := old.Add(-time.Hour)
				tx.CompletedAt, tx.PostedAt = &invalid, from // never hide invalid date with fallback
			}
			insert(t, r, tx)
		}
		report, err := r.TransactionReport(ctx, from, to)
		if err != nil {
			t.Fatal(err)
		}
		if report.Summary.Created.TransactionCount != 0 || report.Summary.Completed.Count != 2 || report.Summary.Completed.TotalAmount != 20000 || len(report.DataQuality) != 2 {
			t.Fatalf("fallback/global errors = %+v", report)
		}
		for _, issue := range report.DataQuality {
			if issue.Scope != "global" || issue.Date != nil || issue.Count != 1 {
				t.Fatalf("wrong global scope: %+v", issue)
			}
		}
	})

	t.Run("one snapshot despite concurrent projection and assessment changes", func(t *testing.T) {
		r := newRepository(t)
		tx := transfer("awaiting_otp", from.Add(time.Hour), 10000)
		tx.OTPExpiresAt = time.Now().Add(time.Hour)
		insert(t, r, tx, assessment(tx, 0.9, "initial"))
		checkpoints := r.transactions.Database().Collection("checkpoints")
		if _, err := checkpoints.InsertOne(ctx, bson.M{"_id": bankCheckpointID, "revision": int64(9007199254740993)}); err != nil {
			t.Fatal(err)
		}
		var requestID atomic.Int64
		var changed atomic.Bool
		monitor := &event.CommandMonitor{
			Started: func(_ context.Context, e *event.CommandStartedEvent) {
				if e.CommandName == "find" && e.Command.Lookup("find").StringValue() == "checkpoints" {
					requestID.Store(e.RequestID)
				}
			},
			Succeeded: func(_ context.Context, e *event.CommandSucceededEvent) {
				if e.CommandName != "find" || e.RequestID != requestID.Load() || !changed.CompareAndSwap(false, true) {
					return
				}
				// Separate client commits after report's first snapshot read and
				// before its transaction/assessment reads. The report must not mix.
				if _, err := r.transactions.UpdateOne(ctx, bson.M{"transaction_id": tx.ID}, bson.M{"$set": bson.M{"status": "cancelled"}}); err != nil {
					t.Error(err)
				}
				if _, err := r.collection.DeleteMany(ctx, bson.M{}); err != nil {
					t.Error(err)
				}
				if _, err := checkpoints.UpdateOne(ctx, bson.M{"_id": bankCheckpointID}, bson.M{"$inc": bson.M{"revision": 1}}); err != nil {
					t.Error(err)
				}
			},
		}
		readerClient, err := mongodrv.Connect(options.Client().ApplyURI(uri).SetDirect(true).SetMonitor(monitor))
		if err != nil {
			t.Fatal(err)
		}
		defer readerClient.Disconnect(context.Background())
		reader := NewRiskAssessmentRepository(readerClient, r.transactions.Database().Name())
		report, err := reader.TransactionReport(ctx, from, to)
		if err != nil {
			t.Fatal(err)
		}
		if !changed.Load() || report.Summary.Created.PendingCount != 1 || report.Summary.Risk.ModelBlockCount != 1 ||
			report.Projection.CheckpointRevision == nil || *report.Projection.CheckpointRevision != "9007199254740993" || report.Projection.Status != "unknown" {
			t.Fatalf("mixed snapshot or unsafe checkpoint serialization: %+v", report)
		}
		after, err := r.TransactionReport(ctx, from, to)
		if err != nil {
			t.Fatal(err)
		}
		if after.Summary.Created.CancelledCount != 1 || after.Summary.Risk.AssessedCount != 0 || *after.Projection.CheckpointRevision != "9007199254740994" {
			t.Fatalf("concurrent update did not actually commit: %+v", after)
		}
	})
}
