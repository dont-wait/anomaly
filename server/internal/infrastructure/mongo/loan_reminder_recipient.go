package mongo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/application/loan/reminder"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// LoanReminderRecipient enforces the ownership relationship already used by
// LoanRepaymentQuery. It does not trust the recipient supplied by a caller.
type LoanReminderRecipient struct{ db *mongodrv.Database }

func NewLoanReminderRecipient(client *mongodrv.Client, dbName string) *LoanReminderRecipient {
	return &LoanReminderRecipient{db: client.Database(dbName)}
}

func (r *LoanReminderRecipient) ResolveEligible(ctx context.Context, item reminder.Candidate) (string, bool, error) {
	if r == nil || r.db == nil {
		return "", false, errors.New("loan reminder recipient query is not configured")
	}
	if strings.TrimSpace(item.LoanID) == "" || item.InstallmentNo <= 0 || item.DueDate.IsZero() || item.TotalDue <= 0 {
		return "", false, errors.New("invalid loan reminder candidate")
	}
	rebuilding, err := (&LoanReadModel{db: r.db}).LoanProjectionRebuilding(ctx)
	if err != nil {
		return "", false, fmt.Errorf("check loan projection readiness: %w", err)
	}
	if rebuilding {
		return "", false, errors.New("loan projection is rebuilding")
	}

	loanOID, applicationOID := loanProjectionIDs(item.LoanID)
	day := item.DueDate.UTC()
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	// Match status, installment number, due date and signed amount in ONE
	// $elemMatch. Previously-paid installments must not produce new alerts.
	filter := bson.M{
		"_id":            loanOID,
		"loan_no":        item.LoanID,
		"application_id": applicationOID,
		"status":         "active",
		"repayment_schedule": bson.M{"$elemMatch": bson.M{
			"installment_no": item.InstallmentNo,
			"status":         bson.M{"$in": reminder.AllowedStatuses(item.ScanAt, item.DueDate)},
			"due_date":       bson.M{"$gte": start, "$lt": start.AddDate(0, 0, 1)},
			"total_due":      item.TotalDue,
		}},
	}
	err = r.db.Collection("loans").FindOne(ctx, filter, options.FindOne().SetProjection(bson.M{"_id": 1})).Err()
	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("recheck upcoming loan installment: %w", err)
	}

	var owner struct {
		OwnerID string `bson:"owner_id"`
	}
	err = r.db.Collection("loan_applications_read").FindOne(ctx, bson.M{
		"_id": item.LoanID, "status": "disbursed",
	}, options.FindOne().SetProjection(bson.M{"owner_id": 1})).Decode(&owner)
	if errors.Is(err, mongodrv.ErrNoDocuments) {
		// A lagging or absent owner projection is NOT a valid reason to drop a
		// pending reminder: return an error so a later scheduler pass retries.
		return "", false, errors.New("loan reminder ownership projection unavailable")
	}
	if err != nil {
		return "", false, fmt.Errorf("lookup loan reminder owner: %w", err)
	}
	if strings.TrimSpace(owner.OwnerID) == "" {
		return "", false, errors.New("loan reminder ownership projection missing owner_id")
	}
	return owner.OwnerID, true, nil
}
