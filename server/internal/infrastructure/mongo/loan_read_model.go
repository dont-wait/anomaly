package mongo

import (
	"context"
	"errors"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/loan"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const loanCheckpointID = "loan-projection-all-v1"

type LoanReadModel struct {
	db *mongodrv.Database
}

func NewLoanReadModel(
	client *mongodrv.Client,
	dbName string,
) *LoanReadModel {
	return &LoanReadModel{
		db: client.Database(dbName),
	}
}

func (m *LoanReadModel) EnsureIndexes(
	ctx context.Context,
) error {
	_, err := m.db.Collection(
		"loan_applications_read",
	).Indexes().CreateMany(
		ctx,
		[]mongodrv.IndexModel{
			{
				Keys: bson.D{
					{Key: "owner_id", Value: 1},
					{Key: "created_at", Value: -1},
					{Key: "_id", Value: -1},
				},
			},
			{
				Keys: bson.D{
					{Key: "created_at", Value: -1},
					{Key: "_id", Value: -1},
				},
			},
		},
	)
	if err != nil {
		return err
	}

	return ensureLoanRepaymentIndexes(ctx, m.db)
}

// Lưu aggregate Loan đã replay
// sang MongoDB Read Model.

func (m *LoanReadModel) Save(
	ctx context.Context,
	app *loan.Application,
	revision uint64,
) error {
	collection := m.db.Collection("loan_applications_read")

	data := bson.M{
		"owner_id":    app.CustomerId,
		"created_at":  app.CreatedAt,
		"status":      app.Status,
		"revision":    revision,
		"application": app,
	}

	// Chỉ tạo document khi chưa tồn tại.
	// Nếu đã tồn tại, không thay đổi dữ liệu.
	_, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": app.Id},
		bson.M{"$setOnInsert": data},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil && !mongodrv.IsDuplicateKeyError(err) {
		return err
	}

	// MongoDB chỉ cập nhật khi revision hiện tại
	// nhỏ hơn revision cần ghi.
	_, err = collection.UpdateOne(
		ctx,
		bson.M{
			"_id": app.Id,
			"$or": bson.A{
				bson.M{
					"revision": bson.M{"$lt": revision},
				},
				bson.M{
					"revision": bson.M{"$exists": false},
				},
			},
		},
		bson.M{"$set": data},
	)
	if err != nil {
		return err
	}

	return m.SaveLoanContract(ctx, app)
}

func (m *LoanReadModel) FindByID(
	ctx context.Context,
	id string,
) (*loan.Application, error) {
	var rec struct {
		Application loan.Application `bson:"application"`
	}

	err := m.db.Collection(
		"loan_applications_read",
	).FindOne(
		ctx,
		bson.M{"_id": id},
	).Decode(&rec)

	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return &rec.Application, nil
}

func (m *LoanReadModel) ListByCustomer(
	ctx context.Context,
	customerID string,
	limit int,
) ([]*loan.Application, error) {
	return m.list(
		ctx,
		bson.M{"owner_id": customerID},
		limit,
	)
}

func (m *LoanReadModel) ListAll(
	ctx context.Context,
	limit int,
) ([]*loan.Application, error) {
	return m.list(
		ctx,
		bson.M{},
		limit,
	)
}

func (m *LoanReadModel) list(
	ctx context.Context,
	filter bson.M,
	limit int,
) ([]*loan.Application, error) {
	if limit <= 0 || limit > 50 {
		limit = 50
	}

	cursor, err := m.db.Collection(
		"loan_applications_read",
	).Find(
		ctx,
		filter,
		options.Find().
			SetSort(
				bson.D{
					{Key: "created_at", Value: -1},
					{Key: "_id", Value: 1},
				},
			).
			SetLimit(int64(limit)),
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	out := make([]*loan.Application, 0)

	for cursor.Next(ctx) {
		var rec struct {
			Application loan.Application `bson:"application"`
		}

		if err := cursor.Decode(&rec); err != nil {
			return nil, err
		}

		a := rec.Application
		out = append(out, &a)
	}

	return out, cursor.Err()
}

// Checkpoint phục vụ Loan Projector.
type LoanCheckpoint struct {
	Commit  uint64    `bson:"commit"`
	Prepare uint64    `bson:"prepare"`
	EventID string    `bson:"event_id,omitempty"`
	SavedAt time.Time `bson:"saved_at"`
}

func (m *LoanReadModel) LoadCheckpoint(
	ctx context.Context,
) (LoanCheckpoint, bool, error) {
	var row LoanCheckpoint

	err := m.db.Collection(
		"checkpoints",
	).FindOne(
		ctx,
		bson.M{"_id": loanCheckpointID},
	).Decode(&row)

	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return LoanCheckpoint{}, false, nil
	}

	return row, err == nil, err
}

func (m *LoanReadModel) SaveCheckpoint(
	ctx context.Context,
	position LoanCheckpoint,
) error {
	position.SavedAt = time.Now().UTC()

	collection := m.db.Collection("checkpoints")

	// Chỉ tạo checkpoint khi chưa tồn tại.
	_, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": loanCheckpointID},
		bson.M{
			"$setOnInsert": position,
		},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil && !mongodrv.IsDuplicateKeyError(err) {
		return err
	}

	// Chỉ cho phép checkpoint tiến về phía trước.
	_, err = collection.UpdateOne(
		ctx,
		bson.M{
			"_id": loanCheckpointID,
			"$or": bson.A{
				bson.M{
					"commit": bson.M{
						"$lt": position.Commit,
					},
				},
				bson.M{
					"commit": position.Commit,
					"prepare": bson.M{
						"$lt": position.Prepare,
					},
				},
			},
		},
		bson.M{
			"$set": position,
		},
	)

	return err
}

func (m *LoanReadModel) ResetAfterInvalidCheckpoint(
	ctx context.Context,
) error {
	// Xóa dữ liệu đọc cũ để xây dựng lại từ EventStoreDB.
	_, err := m.db.Collection(
		"loan_applications_read",
	).DeleteMany(ctx, bson.M{})
	if err != nil {
		return err
	}

	// Giữ lịch sử lỗi RV04 nhưng không để lỗi cũ
	// chặn quá trình replay lại.
	_, err = m.db.Collection(
		"loan_projection_failures",
	).UpdateMany(
		ctx,
		bson.M{"status": "open"},
		bson.M{
			"$set": bson.M{
				"status":      "superseded",
				"resolved_at": time.Now().UTC(),
			},
		},
	)
	if err != nil {
		return err
	}

	// Xóa checkpoint cuối cùng để lần chạy tiếp theo
	// SubscribeToAll bắt đầu từ Start.
	_, err = m.db.Collection("checkpoints").DeleteOne(
		ctx,
		bson.M{"_id": loanCheckpointID},
	)

	return err
}

func (m *LoanReadModel) ListByCustomerPage(
	ctx context.Context,
	customerID string,
	limit int,
	afterTime time.Time,
	afterID string,
) ([]*loan.Application, bool, error) {
	return m.listPage(
		ctx,
		bson.M{"owner_id": customerID},
		limit,
		afterTime,
		afterID,
	)
}

func (m *LoanReadModel) ListAllPage(
	ctx context.Context,
	limit int,
	afterTime time.Time,
	afterID string,
) ([]*loan.Application, bool, error) {
	return m.listPage(
		ctx,
		bson.M{},
		limit,
		afterTime,
		afterID,
	)
}

func (m *LoanReadModel) listPage(
	ctx context.Context,
	filter bson.M,
	limit int,
	afterTime time.Time,
	afterID string,
) ([]*loan.Application, bool, error) {
	if limit <= 0 || limit > 100 {
		return nil, false, errors.New("invalid loan page limit")
	}

	if afterID != "" {
		if afterTime.IsZero() {
			return nil, false, errors.New("invalid loan page cursor")
		}

		filter["$or"] = bson.A{
			bson.M{
				"created_at": bson.M{"$lt": afterTime},
			},
			bson.M{
				"created_at": afterTime,
				"_id":        bson.M{"$lt": afterID},
			},
		}
	}

	cursor, err := m.db.Collection(
		"loan_applications_read",
	).Find(
		ctx,
		filter,
		options.Find().
			SetSort(bson.D{
				{Key: "created_at", Value: -1},
				{Key: "_id", Value: -1},
			}).
			SetLimit(int64(limit+1)),
	)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	items := make([]*loan.Application, 0, limit)
	hasMore := false

	for cursor.Next(ctx) {
		if len(items) == limit {
			hasMore = true
			break
		}

		var row struct {
			Application loan.Application `bson:"application"`
		}

		if err := cursor.Decode(&row); err != nil {
			return nil, false, err
		}

		items = append(items, &row.Application)
	}

	if err := cursor.Err(); err != nil {
		return nil, false, err
	}

	return items, hasMore, nil
}
