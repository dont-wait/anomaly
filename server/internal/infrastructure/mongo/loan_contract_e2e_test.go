//go:build loan_contract_e2e

package mongo

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	loanqueries "github.com/dont-wait/anomaly/internal/application/loan/queries"
	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestLoanContractMongoE2E(t *testing.T) {
	uri := os.Getenv("LOAN_CONTRACT_E2E_MONGO_URI")
	if uri == "" {
		t.Skip("set LOAN_CONTRACT_E2E_MONGO_URI to a TEST MongoDB instance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	client, err := mongodrv.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(context.Background())
	name := "anomaly_loan_contract_e2e_" + uuid.NewString()[:8]
	db := client.Database(name)
	defer db.Drop(context.Background())

	read := NewLoanReadModel(client, name)
	if err := read.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	service := loanqueries.NewLoanContractQueryService(NewLoanContractQuery(read))
	owner := uuid.NewString()
	outsider := uuid.NewString()
	base := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	firstID, secondID := uuid.NewString(), uuid.NewString()
	rate, err := bson.ParseDecimal128("16.70")
	if err != nil {
		t.Fatal(err)
	}

	seed := func(id string, applicationCreatedAt time.Time, status string, outstanding int64, paid int) {
		t.Helper()
		app := loandomain.Application{
			Id: id, CustomerId: owner, ApplicationNo: "APP-" + id,
			LoanPackageId: "PERSONAL_UNSECURED_SALARY",
			Status:        loandomain.ApplicationStatusDisbursed,
			CreatedAt:     applicationCreatedAt,
		}
		_, err := db.Collection("loan_applications_read").InsertOne(ctx, bson.M{
			"_id": id, "owner_id": owner, "status": "disbursed",
			"created_at": applicationCreatedAt, "application": app,
		})
		if err != nil {
			t.Fatal(err)
		}
		loanOID, appOID := loanProjectionIDs(id)
		items := make([]repaymentInstallmentRow, 0, 2)
		for n := 1; n <= 2; n++ {
			state := "upcoming"
			if n <= paid {
				state = "paid"
			}
			items = append(items, repaymentInstallmentRow{
				InstallmentNo: n, DueDate: base.AddDate(0, n, 0),
				PrincipalDue: 5_000_000, InterestDue: 100_000,
				TotalDue: 5_100_000, Status: state,
			})
		}
		_, err = db.Collection("loans").InsertOne(ctx, bson.M{
			"_id": loanOID, "loan_no": id, "application_id": appOID,
			"principal": int64(10_000_000), "outstanding_principal": outstanding,
			"interest_rate": rate, "term_months": 2, "status": status,
			"start_date": base, "maturity_date": items[1].DueDate,
			"repayment_schedule": items,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	seed(firstID, base.AddDate(0, 0, -2), "completed", 0, 2)
	seed(secondID, base.AddDate(0, 0, -1), "active", 10_000_000, 0)

	page, err := service.List(ctx, owner, 1, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].LoanID != secondID || page.NextCursor == nil {
		t.Fatalf("first page: %+v, err=%v", page, err)
	}
	next, err := service.List(ctx, owner, 1, *page.NextCursor)
	if err != nil || len(next.Items) != 1 || next.Items[0].LoanID != firstID || next.NextCursor != nil {
		t.Fatalf("second page: %+v, err=%v", next, err)
	}
	if _, err := service.List(ctx, outsider, 1, *page.NextCursor); !errors.Is(err, loanqueries.ErrInvalidLoanContractQuery) {
		t.Fatalf("owner cursor reused by outsider: %v", err)
	}

	detail, err := service.Get(ctx, firstID, owner)
	if err != nil || detail.Status != "completed" || detail.OutstandingPrincipal != 0 ||
		detail.PaidInstallments != 2 || detail.NextInstallmentNo != 0 || len(detail.Schedule) != 2 {
		t.Fatalf("completed contract: %+v, err=%v", detail, err)
	}
	if _, err := service.Get(ctx, firstID, outsider); !errors.Is(err, loanqueries.ErrLoanContractNotFound) {
		t.Fatalf("outsider could read contract: %v", err)
	}
	if _, err := service.Get(ctx, uuid.NewString(), owner); !errors.Is(err, loanqueries.ErrLoanContractNotFound) {
		t.Fatalf("missing contract leaked: %v", err)
	}

	loanOID, _ := loanProjectionIDs(secondID)
	_, err = db.Collection("loans").UpdateOne(ctx, bson.M{"_id": loanOID}, bson.M{"$unset": bson.M{"repayment_schedule": ""}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, secondID, owner); !errors.Is(err, loanqueries.ErrLoanContractNotReady) {
		t.Fatalf("missing schedule not detected: %v", err)
	}
	if _, err := service.List(ctx, owner, 20, ""); !errors.Is(err, loanqueries.ErrLoanContractNotReady) {
		t.Fatalf("incomplete list silently omitted contract: %v", err)
	}
}
