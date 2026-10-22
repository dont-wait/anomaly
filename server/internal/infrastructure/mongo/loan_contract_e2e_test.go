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

	seed := func(
		id string,
		customerID string,
		applicationCreatedAt time.Time,
		status string,
		outstanding int64,
		paid int,
	) {
		t.Helper()

		app := loandomain.Application{
			Id:              id,
			CustomerId:      customerID,
			ApplicationNo:   "APP-" + id,
			LoanPackageId:   "PERSONAL_UNSECURED_SALARY",
			RequestedAmount: 10_000_000,
			TermMonths:      2,
			Status:          loandomain.ApplicationStatusDisbursed,
			CreatedAt:       applicationCreatedAt,
		}

		_, err := db.Collection("loan_applications_read").InsertOne(ctx, bson.M{
			"_id": id, "owner_id": customerID, "status": "disbursed",
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

	foreignID := uuid.NewString()

	seed(firstID, owner, base.AddDate(0, 0, -2), "completed", 0, 2)
	seed(secondID, owner, base.AddDate(0, 0, -1), "active", 10_000_000, 0)

	// The foreign contract is newer than both owned contracts.
	seed(foreignID, outsider, base, "active", 10_000_000, 0)

	page, err := service.List(ctx, owner, 1, "")
	if err != nil ||
		len(page.Items) != 1 ||
		page.Items[0].LoanID != secondID ||
		page.NextCursor == nil {
		t.Fatalf("first page: %+v, err=%v", page, err)
	}

	// A foreign disbursed contract must never appear in the owner's list.
	allOwned, err := service.List(ctx, owner, 20, "")
	if err != nil {
		t.Fatalf("list owned contracts: %v", err)
	}

	if len(allOwned.Items) != 2 {
		t.Fatalf(
			"expected exactly 2 owned contracts, got %d",
			len(allOwned.Items),
		)
	}

	for _, item := range allOwned.Items {
		if item.LoanID == foreignID {
			t.Fatal("foreign customer's contract leaked into owner list")
		}
	}

	// The foreign customer must not see the owner's contracts.
	foreignPage, err := service.List(ctx, outsider, 20, "")
	if err != nil {
		t.Fatalf("list foreign customer contracts: %v", err)
	}

	if len(foreignPage.Items) != 1 ||
		foreignPage.Items[0].LoanID != foreignID {
		t.Fatalf(
			"foreign customer received incorrect contracts: %+v",
			foreignPage,
		)
	}

	next, err := service.List(ctx, owner, 1, *page.NextCursor)
	if err != nil || len(next.Items) != 1 || next.Items[0].LoanID != firstID || next.NextCursor != nil {
		t.Fatalf("second page: %+v, err=%v", next, err)
	}

	// RV8: An older application enters the disbursed set between pages.
	lateID := uuid.NewString()
	lateCreatedAt := base.AddDate(0, 0, -3)
	seed(lateID, owner, lateCreatedAt, "active", 10_000_000, 0)

	// Keep the contract out of the first-page snapshot initially.
	lateApp := loandomain.Application{
		Id:              lateID,
		CustomerId:      owner,
		ApplicationNo:   "APP-" + lateID,
		LoanPackageId:   "PERSONAL_UNSECURED_SALARY",
		RequestedAmount: 10_000_000,
		TermMonths:      2,
		Status:          loandomain.ApplicationStatusApproved,
		CreatedAt:       lateCreatedAt,
	}
	_, err = db.Collection("loan_applications_read").UpdateOne(
		ctx,
		bson.M{"_id": lateID},
		bson.M{"$set": bson.M{
			"status":      string(loandomain.ApplicationStatusApproved),
			"application": lateApp,
			"revision":    int64(1),
		}},
	)
	if err != nil {
		t.Fatalf("prepare approved application: %v", err)
	}

	beforeChange, err := service.List(ctx, owner, 1, "")
	if err != nil || beforeChange.NextCursor == nil {
		t.Fatalf("first page before disbursement: %+v, err=%v", beforeChange, err)
	}

	// A previously approved older application is now disbursed.
	lateApp.Status = loandomain.ApplicationStatusDisbursed
	_, err = db.Collection("loan_applications_read").UpdateOne(
		ctx,
		bson.M{"_id": lateID},
		bson.M{"$set": bson.M{
			"status":      string(loandomain.ApplicationStatusDisbursed),
			"application": lateApp,
			"revision":    int64(2),
		}},
	)
	if err != nil {
		t.Fatalf("disburse older application: %v", err)
	}

	_, err = service.List(ctx, owner, 1, *beforeChange.NextCursor)
	if !errors.Is(err, loanqueries.ErrLoanContractPageChanged) {
		t.Fatalf("expected changed list to reject old cursor, got %v", err)
	}

	restarted, err := service.List(ctx, owner, 20, "")
	if err != nil {
		t.Fatalf("restart list after disbursement: %v", err)
	}
	if len(restarted.Items) != 3 {
		t.Fatalf("expected 3 owner contracts, got %d", len(restarted.Items))
	}
	seenLate := false
	for _, item := range restarted.Items {
		if item.LoanID == lateID {
			seenLate = true
		}
		if item.LoanID == foreignID {
			t.Fatal("foreign contract leaked after pagination restart")
		}
	}
	if !seenLate {
		t.Fatal("newly disbursed contract missing after pagination restart")
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

	// RV6: recovery must return retryable errors, not an empty list/404.
	_, err = db.Collection("loan_projection_state").InsertOne(ctx, bson.M{
		"_id": loanRecoveryStateID, "status": "rebuilding",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.List(ctx, owner, 20, ""); !errors.Is(err, loanqueries.ErrLoanContractNotReady) {
		t.Fatalf("list during recovery: %v", err)
	}
	if _, err := service.Get(ctx, uuid.NewString(), owner); !errors.Is(err, loanqueries.ErrLoanContractNotReady) {
		t.Fatalf("detail during recovery: %v", err)
	}
}
