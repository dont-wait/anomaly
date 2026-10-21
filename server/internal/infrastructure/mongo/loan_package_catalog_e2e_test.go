//go:build loan_products_e2e

package mongo

import (
	"context"
	"os"
	"testing"
	"time"

	loanqueries "github.com/dont-wait/anomaly/internal/application/loan/queries"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestLoanPackageCatalogMongoE2E(t *testing.T) {
	uri := os.Getenv("LOAN_PRODUCTS_E2E_MONGO_URI")
	if uri == "" {
		t.Skip("set LOAN_PRODUCTS_E2E_MONGO_URI to a dedicated local TEST MongoDB")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	client, err := mongodrv.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(context.Background())
	dbName := "anomaly_loan_products_e2e_" + uuid.NewString()[:8]
	db := client.Database(dbName)
	defer db.Drop(context.Background())

	collection := db.Collection("loan_packages")
	if _, err := collection.InsertMany(ctx, []any{
		bson.M{
			"package_code": "PERSONAL_UNSECURED_SALARY", "name": "Vay tín chấp theo lương",
			"min_amount": int64(1_000_000), "max_amount": int64(500_000_000), "status": "active",
			"term_options": bson.A{bson.M{"months": 6}, bson.M{"months": 12}, bson.M{"months": 24}},
		},
		bson.M{
			"package_code": "OLD_PACKAGE", "name": "Old package", "min_amount": int64(1_000_000),
			"max_amount": int64(2_000_000), "status": "active", "term_options": bson.A{bson.M{"months": 12}},
		},
		bson.M{
			"package_code": "PERSONAL_UNSECURED_SALARY", "name": "Inactive",
			"min_amount": int64(1_000_000), "max_amount": int64(2_000_000),
			"status": "inactive", "term_options": bson.A{bson.M{"months": 12}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	catalog := NewLoanPackageCatalog(client, dbName)
	products, err := catalog.ListActiveLoanPackages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(products) != 1 || products[0].PackageCode != "PERSONAL_UNSECURED_SALARY" || len(products[0].TermMonths) != 3 {
		t.Fatalf("unexpected active packages: %+v", products)
	}
	quote := loanqueries.NewPreApplicationQuoteQueryService(catalog)
	result, err := quote.Quote(ctx, loanqueries.PreApplicationQuoteInput{
		LoanPackageID: "PERSONAL_UNSECURED_SALARY", RequestedAmount: 50_000_000,
		TermMonths: 12, MonthlyIncome: 15_000_000, RegionCode: "KV01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.AnnualRateBps != 1610 || result.Official || len(result.ProvisionalSchedule) != 12 {
		t.Fatalf("invalid Mongo-backed quote: %+v", result)
	}
}
