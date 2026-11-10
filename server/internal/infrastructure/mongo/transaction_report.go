package mongo

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/dont-wait/anomaly/internal/application/reporting"
	"github.com/dont-wait/anomaly/internal/domain/riskassessment"
	"github.com/dont-wait/anomaly/internal/timeutil"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

var _ reporting.Reader = (*RiskAssessmentRepository)(nil)

// TransactionReport reads checkpoint, both periods, and assessments in the
// same snapshot. It never falls back to unrelated non-transactional reads.
func (r *RiskAssessmentRepository) TransactionReport(ctx context.Context, from, to time.Time) (reporting.Report, error) {
	session, err := r.client.StartSession()
	if err != nil {
		return reporting.Report{}, err
	}
	defer session.EndSession(ctx)
	result, err := session.WithTransaction(ctx, func(sc context.Context) (any, error) {
		var checkpoint *string
		var row struct {
			Revision uint64 `bson:"revision"`
		}
		err := r.transactions.Database().Collection("checkpoints").FindOne(sc, bson.M{"_id": bankCheckpointID}).Decode(&row)
		if err != nil && err != mongodrv.ErrNoDocuments {
			return nil, err
		}
		if err == nil {
			value := strconv.FormatUint(row.Revision, 10)
			checkpoint = &value
		}
		// The first read establishes the snapshot before selecting the wall-clock
		// reference. Retried transactions get a new builder and reference time.
		asOf := time.Now().UTC()
		builder := reporting.NewBuilder(from, to, asOf, checkpoint)
		previousFrom := from.In(timeutil.VietnamLocation).AddDate(0, 0, -1).UTC()
		created, err := r.reportCreatedTransfers(sc, previousFrom, to, asOf)
		if err != nil {
			return nil, err
		}
		assessments, err := r.reportAssessments(sc, created)
		if err != nil {
			return nil, err
		}
		for _, tx := range created {
			if err := builder.AddCreated(reportTransfer(tx), assessments[tx.ID]); err != nil {
				return nil, err
			}
		}
		cursor, err := r.transactions.Find(sc, reportCompletionFilter(previousFrom, to, asOf))
		if err != nil {
			return nil, err
		}
		defer func() { _ = cursor.Close(sc) }()
		for cursor.Next(sc) {
			var tx TransferDocument
			if err := cursor.Decode(&tx); err != nil {
				return nil, err
			}
			if err := builder.AddCompleted(reportTransfer(tx)); err != nil {
				return nil, err
			}
		}
		if err := cursor.Err(); err != nil {
			return nil, err
		}
		return builder.Finish(time.Now().UTC()), nil
	}, options.Transaction().SetReadConcern(readconcern.Snapshot()).SetReadPreference(readpref.Primary()).SetWriteConcern(writeconcern.Majority()))
	if err != nil {
		return reporting.Report{}, fmt.Errorf("read transaction report snapshot: %w", err)
	}
	return result.(reporting.Report), nil
}

func (r *RiskAssessmentRepository) reportCreatedTransfers(ctx context.Context, from, to, asOf time.Time) ([]TransferDocument, error) {
	cursor, err := r.transactions.Find(ctx, bson.M{
		"type": "transfer", "created_at": bson.M{"$gte": from, "$lt": to, "$lte": asOf},
	}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "transaction_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	var transfers []TransferDocument
	if err := cursor.All(ctx, &transfers); err != nil {
		return nil, err
	}
	return transfers, nil
}

func (r *RiskAssessmentRepository) reportAssessments(ctx context.Context, transfers []TransferDocument) (map[string][]riskassessment.Assessment, error) {
	result := make(map[string][]riskassessment.Assessment, len(transfers))
	// Batch by transaction, not by recent assessment count. All model/feature
	// versions remain eligible, including finalized/handled historical records.
	const batchSize = 500
	for start := 0; start < len(transfers); start += batchSize {
		end := min(start+batchSize, len(transfers))
		ids := make([]string, 0, end-start)
		for _, tx := range transfers[start:end] {
			ids = append(ids, tx.ID)
		}
		cursor, err := r.collection.Find(ctx, bson.M{
			"transaction_id": bson.M{"$in": ids},
		})
		if err != nil {
			return nil, err
		}
		for cursor.Next(ctx) {
			var assessment riskassessment.Assessment
			if err := cursor.Decode(&assessment); err != nil {
				_ = cursor.Close(ctx)
				return nil, err
			}
			result[assessment.TransactionID] = append(result[assessment.TransactionID], assessment)
		}
		cursorErr := cursor.Err()
		_ = cursor.Close(ctx)
		if cursorErr != nil {
			return nil, cursorErr
		}
	}
	return result, nil
}

// Limit completion rows to the two periods plus globally unassignable dates.
// The latter must not disappear just because their created_at is outside D/D-1.
// Null/missing/Go-zero completed_at falls back to posted_at; an invalid
// nonzero completed_at never does. No full success-history slice is retained.
func reportCompletionFilter(from, to, asOf time.Time) bson.M {
	zero := time.Time{}
	effective := bson.M{"$cond": bson.A{
		bson.M{"$eq": bson.A{bson.M{"$ifNull": bson.A{"$completed_at", zero}}, zero}},
		bson.M{"$ifNull": bson.A{"$posted_at", zero}},
		"$completed_at",
	}}
	return bson.M{
		"type": "transfer", "status": "success", "created_at": bson.M{"$lte": asOf},
		"$expr": bson.M{"$let": bson.M{
			"vars": bson.M{"completed": effective},
			"in": bson.M{"$or": bson.A{
				bson.M{"$eq": bson.A{"$$completed", zero}},
				bson.M{"$lt": bson.A{"$$completed", "$created_at"}},
				bson.M{"$and": bson.A{
					bson.M{"$gte": bson.A{"$$completed", from}},
					bson.M{"$lt": bson.A{"$$completed", to}},
					bson.M{"$lte": bson.A{"$$completed", asOf}},
				}},
			}},
		}},
	}
}

func reportTransfer(tx TransferDocument) reporting.Transfer {
	return reporting.Transfer{
		ID: tx.ID, SourceAccountNo: tx.Source.AccountNo, SourceName: tx.Source.Name,
		DestinationAccountNo: tx.Destination.AccountNo, DestinationName: tx.Destination.Name,
		Amount: tx.Amount, Fee: tx.Fee, Currency: tx.Currency, Status: tx.Status,
		FailureReason: tx.FailureReason, CreatedAt: tx.CreatedAt, PostedAt: tx.PostedAt,
		OTPExpiresAt: tx.OTPExpiresAt, CompletedAt: tx.CompletedAt, FailedAt: tx.FailedAt,
		CancelledAt: tx.CancelledAt, RiskAssessment: tx.RiskAssessment,
	}
}
