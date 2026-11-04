package mongo

import (
	"context"
	"fmt"
	"time"

	"github.com/dont-wait/anomaly/internal/application/loan/reminder"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// LoanReminderQuery reads the projected EMI schedule only. It never writes
// repayments, customer balances, or EventStoreDB events.
type LoanReminderQuery struct {
	col *mongodrv.Collection
}

func NewLoanReminderQuery(client *mongodrv.Client, dbName string) *LoanReminderQuery {
	return &LoanReminderQuery{col: client.Database(dbName).Collection("loans")}
}

type reminderLoanDocument struct {
	LoanNo   string `bson:"loan_no"`
	Status   string `bson:"status"`
	Schedule []struct {
		InstallmentNo int       `bson:"installment_no"`
		DueDate       time.Time `bson:"due_date"`
		TotalDue      int64     `bson:"total_due"`
		Status        string    `bson:"status"`
	} `bson:"repayment_schedule"`
}

func (q *LoanReminderQuery) FindUpcoming(ctx context.Context, datesUTC []time.Time) ([]reminder.Installment, error) {
	if q == nil || q.col == nil {
		return nil, fmt.Errorf("loan reminder MongoDB repository is not configured")
	}
	if len(datesUTC) == 0 {
		return nil, nil
	}

	// Each due_date in the signed EMI schedule is UTC midnight, not a UTC
	// timestamp to be shifted to local midnight. Match UTC calendar days.
	or := make(bson.A, 0, len(datesUTC))
	wanted := make(map[string]struct{}, len(datesUTC))
	for _, day := range datesUTC {
		start := time.Date(day.UTC().Year(), day.UTC().Month(), day.UTC().Day(), 0, 0, 0, 0, time.UTC)
		or = append(or, bson.M{"repayment_schedule": bson.M{"$elemMatch": bson.M{
			"status":   "upcoming",
			"due_date": bson.M{"$gte": start, "$lt": start.AddDate(0, 0, 1)},
		}}})
		wanted[start.Format("2006-01-02")] = struct{}{}
	}
	filter := bson.M{"status": "active", "$or": or}
	cursor, err := q.col.Find(ctx, filter, options.Find().SetProjection(bson.M{
		"loan_no": 1, "status": 1, "repayment_schedule": 1,
	}))
	if err != nil {
		return nil, fmt.Errorf("query loan reminders: %w", err)
	}
	defer cursor.Close(ctx)

	var result []reminder.Installment
	for cursor.Next(ctx) {
		var loan reminderLoanDocument
		if err := cursor.Decode(&loan); err != nil {
			return nil, fmt.Errorf("decode loan reminder: %w", err)
		}
		for _, item := range loan.Schedule {
			if item.Status != "upcoming" {
				continue
			}
			if _, ok := wanted[item.DueDate.UTC().Format("2006-01-02")]; !ok {
				continue
			}
			result = append(result, reminder.Installment{
				LoanID: loan.LoanNo, LoanNo: loan.LoanNo, LoanStatus: loan.Status,
				InstallmentNo: item.InstallmentNo, DueDate: item.DueDate,
				TotalDue: item.TotalDue, Status: item.Status,
			})
		}
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("iterate loan reminders: %w", err)
	}
	return result, nil
}
