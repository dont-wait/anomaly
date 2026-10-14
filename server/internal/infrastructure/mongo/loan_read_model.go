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
				},
			},
			{
				Keys: bson.D{
					{Key: "created_at", Value: -1},
				},
			},
		},
	)

	return err
}

// Lưu aggregate Loan đã replay
// sang MongoDB Read Model.
func (m *LoanReadModel) Save(
	ctx context.Context,
	app *loan.Application,
	revision uint64,
) error {
	_, err := m.db.Collection(
		"loan_applications_read",
	).UpdateOne(
		ctx,
		bson.M{"_id": app.Id},
		bson.M{
			"$set": bson.M{
				"owner_id":    app.CustomerId,
				"created_at":  app.CreatedAt,
				"status":      app.Status,
				"revision":    revision,
				"application": app,
			},
		},
		options.UpdateOne().SetUpsert(true),
	)

	return err
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
	defer cursor.Close(ctx)

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

	_, err := m.db.Collection(
		"checkpoints",
	).UpdateOne(
		ctx,
		bson.M{"_id": loanCheckpointID},
		bson.M{"$set": position},
		options.UpdateOne().SetUpsert(true),
	)

	return err
}
