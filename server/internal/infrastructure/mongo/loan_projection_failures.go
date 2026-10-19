package mongo

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const loanProjectionFailures = "loan_projection_failures"

// Tạo index để quản trị viên tìm các lỗi chưa xử lý.
func (m *LoanReadModel) EnsureFailureIndexes(ctx context.Context) error {
	_, err := m.db.Collection(loanProjectionFailures).Indexes().CreateOne(
		ctx,
		mongodrv.IndexModel{
			Keys: bson.D{
				{Key: "status", Value: 1},
				{Key: "last_seen_at", Value: -1},
			},
		},
	)
	return err
}

// Kiểm tra hồ sơ đang bị cách ly hay không.
func (m *LoanReadModel) HasOpenProjectionFailure(
	ctx context.Context,
	loanID string,
) (bool, error) {
	count, err := m.db.Collection(loanProjectionFailures).
		CountDocuments(ctx, bson.M{
			"_id":    loanID,
			"status": "open",
		})

	return count > 0, err
}

// Lưu lỗi bền vững trước khi cho checkpoint đi tiếp.
func (m *LoanReadModel) RecordProjectionFailure(
	ctx context.Context,
	loanID string,
	position LoanCheckpoint,
	cause error,
) error {
	if loanID == "" || cause == nil {
		return fmt.Errorf("invalid loan projection failure")
	}

	now := time.Now().UTC()

	_, err := m.db.Collection(loanProjectionFailures).UpdateOne(
		ctx,
		bson.M{"_id": loanID},
		bson.M{
			"$set": bson.M{
				"stream_id":    "loan-application-" + loanID,
				"status":       "open",
				"reason":       cause.Error(),
				"last_commit":  position.Commit,
				"last_prepare": position.Prepare,
				"last_seen_at": now,
			},
			"$setOnInsert": bson.M{
				"first_seen_at": now,
				"first_commit":  position.Commit,
				"first_prepare": position.Prepare,
			},
			"$inc": bson.M{
				"attempts": int64(1),
			},
			"$unset": bson.M{
				"resolved_at": "",
			},
		},
		options.UpdateOne().SetUpsert(true),
	)

	return err
}

// Chỉ đánh dấu đã xử lý sau khi replay và lưu Read Model thành công.
func (m *LoanReadModel) ResolveProjectionFailure(
	ctx context.Context,
	loanID string,
) error {
	now := time.Now().UTC()

	result, err := m.db.Collection(loanProjectionFailures).UpdateOne(
		ctx,
		bson.M{
			"_id":    loanID,
			"status": "open",
		},
		bson.M{
			"$set": bson.M{
				"status":      "resolved",
				"resolved_at": now,
			},
		},
	)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("no open projection failure for loan %s", loanID)
	}

	return nil
}
