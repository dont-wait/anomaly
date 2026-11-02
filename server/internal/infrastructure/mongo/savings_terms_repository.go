package mongo

import (
	"context"
	"fmt"
	"time"

	savingsdomain "github.com/dont-wait/anomaly/internal/domain/savings"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
)

// SavingsTermsRepository reads the published product disclosure from MongoDB.
// It intentionally does not seed or substitute content: publishing belongs to
// the versioned database migration / product content release process.
type SavingsTermsRepository struct {
	collection *mongodrv.Collection
}

func NewSavingsTermsRepository(client *mongodrv.Client, databaseName string) *SavingsTermsRepository {
	return &SavingsTermsRepository{collection: client.Database(databaseName).Collection("savings_terms")}
}

func (r *SavingsTermsRepository) Current(ctx context.Context) (savingsdomain.TermsSnapshot, error) {
	if r == nil || r.collection == nil {
		return savingsdomain.TermsSnapshot{}, savingsdomain.ErrTermsUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var row struct {
		ProductCode  string                       `bson:"product_code"`
		Status       string                       `bson:"status"`
		Version      string                       `bson:"version"`
		Title        string                       `bson:"title"`
		Introduction string                       `bson:"introduction"`
		Sections     []savingsdomain.TermsSection `bson:"sections"`
	}
	cursor, err := r.collection.Find(ctx, bson.M{
		"product_code": "savings_standard", "status": "ACTIVE",
	})
	if err != nil {
		return savingsdomain.TermsSnapshot{}, fmt.Errorf("query savings terms: %w", err)
	}
	defer cursor.Close(ctx)
	if !cursor.Next(ctx) {
		if cursor.Err() != nil {
			return savingsdomain.TermsSnapshot{}, cursor.Err()
		}
		return savingsdomain.TermsSnapshot{}, savingsdomain.ErrTermsUnavailable
	}
	if err := cursor.Decode(&row); err != nil {
		return savingsdomain.TermsSnapshot{}, fmt.Errorf("decode savings terms: %w", err)
	}
	if cursor.Next(ctx) {
		return savingsdomain.TermsSnapshot{}, fmt.Errorf("multiple ACTIVE savings terms for savings_standard: %w", savingsdomain.ErrTermsUnavailable)
	}
	if err := cursor.Err(); err != nil {
		return savingsdomain.TermsSnapshot{}, fmt.Errorf("read active savings terms: %w", err)
	}
	if row.ProductCode != "savings_standard" || row.Status != "ACTIVE" {
		return savingsdomain.TermsSnapshot{}, savingsdomain.ErrTermsUnavailable
	}
	content := savingsdomain.TermsSnapshot{
		Version: row.Version, Title: row.Title, Introduction: row.Introduction, Sections: row.Sections,
	}
	if err := content.Validate(); err != nil {
		return savingsdomain.TermsSnapshot{}, err
	}
	return content, nil
}
