package mongo

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/application/loan/reminder"
	notificationapp "github.com/dont-wait/anomaly/internal/application/notification"
	"github.com/dont-wait/anomaly/internal/domain/notification"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Set LOAN_REMINDER_E2E_MONGO_URI only to a TEST MongoDB instance.
// This test uses an isolated database and drops it on exit.
func TestLoanReminderNotificationE2E(t *testing.T) {
	uri := os.Getenv("LOAN_REMINDER_E2E_MONGO_URI")
	if uri == "" {
		t.Skip("set LOAN_REMINDER_E2E_MONGO_URI to a test MongoDB")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	client, err := mongodrv.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			t.Errorf("disconnect MongoDB E2E client: %v", err)
		}
	}()
	dbName := "anomaly_loan_reminder_e2e_" + uuid.NewString()[:8]
	db := client.Database(dbName)
	defer func() {
		if err := db.Drop(context.Background()); err != nil {
			t.Errorf("drop MongoDB E2E database: %v", err)
		}
	}()

	store := NewNotificationRepository(client, dbName)
	if err := store.CheckReady(ctx); err == nil {
		t.Fatal("missing notifications collection should fail readiness")
	}
	if err := db.CreateCollection(ctx, "notifications"); err != nil {
		t.Fatal(err)
	}
	if err := store.CheckReady(ctx); err == nil {
		t.Fatal("notifications collection without migration indexes should fail readiness")
	}
	if err := store.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.CheckReady(ctx); err != nil {
		t.Fatalf("read-only notification readiness check: %v", err)
	}

	now := time.Now().UTC()
	today, five := reminder.TargetDates(now)
	loanID := "loan-e2e-" + uuid.NewString()
	loanOID, appOID := loanProjectionIDs(loanID)
	const userID = "e2e-jwt-user"
	_, err = db.Collection("loans").InsertOne(ctx, bson.M{
		"_id": loanOID, "loan_no": loanID, "application_id": appOID,
		"status": "active",
		"repayment_schedule": bson.A{
			bson.M{"installment_no": 1, "due_date": today, "total_due": int64(1000000), "status": "upcoming"},
			bson.M{"installment_no": 2, "due_date": five, "total_due": int64(1000000), "status": "upcoming"},
			bson.M{"installment_no": 3, "due_date": today.AddDate(0, 0, 3), "total_due": int64(1000000), "status": "upcoming"},
			bson.M{"installment_no": 4, "due_date": today.AddDate(0, 0, -1), "total_due": int64(1000000), "status": "overdue"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Collection("loan_applications_read").InsertOne(ctx, bson.M{
		"_id": loanID, "owner_id": userID, "status": "disbursed",
	})
	if err != nil {
		t.Fatal(err)
	}

	sender := notificationapp.NewService(NewNotificationRepository(client, dbName), notificationapp.Config{
		Retention: 90 * 24 * time.Hour, MaxEventAge: 24 * time.Hour,
	})
	job := reminder.Job{
		Repository: NewLoanReminderQuery(client, dbName),
		Handler: reminder.NotificationHandler{
			Lookup:   NewLoanReminderRecipient(client, dbName),
			Notifier: sender,
		},
	}
	for i := 0; i < 2; i++ {
		stats, err := job.Run(ctx, time.Now().UTC())
		if err != nil || stats.Matched != 4 || stats.Failed != 0 {
			t.Fatalf("run %d stats=%+v err=%v", i, stats, err)
		}
	}
	count, err := db.Collection("notifications").CountDocuments(ctx, bson.M{
		"recipient_id": userID, "type": string(notification.TypeLoanRepaymentDue),
	})
	if err != nil || count != 4 {
		t.Fatalf("expected T-0, T-5 and two caught-up reminders, count=%d err=%v", count, err)
	}
	for _, kind := range []reminder.ReminderType{reminder.ReminderDueToday, reminder.ReminderFiveDays} {
		installment := 1
		if kind == reminder.ReminderFiveDays {
			installment = 2
		}
		source := reminder.Candidate{LoanID: loanID, InstallmentNo: installment, Type: kind}.IdempotencyKey()
		id := notification.NotificationID(source, notification.TypeLoanRepaymentDue, userID)
		if err := db.Collection("notifications").FindOne(ctx, bson.M{"_id": id, "category": "LOAN"}).Err(); err != nil {
			t.Fatalf("missing %s: %v", kind, err)
		}
	}

	// Catch-up uses the original T-5 deterministic id rather than T-3.
	catchUpSource := (reminder.Candidate{LoanID: loanID, InstallmentNo: 3, Type: reminder.ReminderFiveDays}).IdempotencyKey()
	catchUpID := notification.NotificationID(catchUpSource, notification.TypeLoanRepaymentDue, userID)
	if err := db.Collection("notifications").FindOne(ctx, bson.M{"_id": catchUpID}).Err(); err != nil {
		t.Fatalf("missing caught-up reminder: %v", err)
	}

	// A late unpaid installment uses the stable T-0 id even though it is T+1.
	lateSource := (reminder.Candidate{LoanID: loanID, InstallmentNo: 4, Type: reminder.ReminderDueToday}).IdempotencyKey()
	lateID := notification.NotificationID(lateSource, notification.TypeLoanRepaymentDue, userID)
	if err := db.Collection("notifications").FindOne(ctx, bson.M{"_id": lateID}).Err(); err != nil {
		t.Fatalf("missing late due reminder: %v", err)
	}

	// Verify that a previously paid installment is excluded from re-send.
	_, err = db.Collection("loans").UpdateOne(ctx, bson.M{"_id": loanOID, "repayment_schedule.installment_no": 1}, bson.M{
		"$set": bson.M{"repayment_schedule.$.status": "paid"},
	})
	if err != nil {
		t.Fatal(err)
	}
	firstID := notification.NotificationID(
		(reminder.Candidate{LoanID: loanID, InstallmentNo: 1, Type: reminder.ReminderDueToday}).IdempotencyKey(),
		notification.TypeLoanRepaymentDue, userID,
	)
	if _, err := db.Collection("notifications").DeleteOne(ctx, bson.M{"_id": firstID}); err != nil {
		t.Fatal(err)
	}
	stats, err := job.Run(ctx, time.Now().UTC())
	if err != nil || stats.Matched != 3 {
		t.Fatalf("paid installment still matched: %+v %v", stats, err)
	}
	count, err = db.Collection("notifications").CountDocuments(ctx, bson.M{})
	if err != nil || count != 3 {
		t.Fatalf("paid installment was notified again: %d %v", count, err)
	}
}
