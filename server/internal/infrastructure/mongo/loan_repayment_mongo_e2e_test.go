//go:build repayment_e2e

package mongo

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	loanqueries "github.com/dont-wait/anomaly/internal/application/loan/queries"
	"github.com/dont-wait/anomaly/internal/domain/loan"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/dont-wait/anomaly/internal/timeutil"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mongodb"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestLoanRepaymentMongoProjectionE2E(t *testing.T) {
	uri := os.Getenv("REPAYMENT_E2E_MONGO_URI")
	if uri == "" {
		t.Skip("set REPAYMENT_E2E_MONGO_URI to a TEST MongoDB replica set")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		2*time.Minute,
	)
	defer cancel()

	dbName := "anomaly_repayment_e2e_" +
		uuid.NewString()[:8]

	client, err := mongodrv.Connect(
		options.Client().ApplyURI(uri),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(context.Background())

	db := client.Database(dbName)
	defer db.Drop(context.Background())

	// Chay migration production tren DB test rieng.
	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/" + dbName

	migrationsDir, err := filepath.Abs(
		filepath.Join("..", "..", "..", "migrations"),
	)
	if err != nil {
		t.Fatal(err)
	}

	src, err := iofs.New(
		os.DirFS(migrationsDir), ".",
	)
	if err != nil {
		t.Fatal(err)
	}

	runner, err := migrate.NewWithSourceInstance(
		"iofs", src, parsed.String(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()

	if err := runner.Up(); err != nil {
		t.Fatalf("migrate test DB: %v", err)
	}

	projection := NewBankProjection(client, dbName)

	if err := projection.EnsureLoanRepaymentIndexes(
		ctx,
	); err != nil {
		t.Fatal(err)
	}

	// Tao tai khoan gia lap hop le.
	accountID := bson.NewObjectID()
	customerID := bson.NewObjectID()

	at := time.Date(
		2026, 10, 15, 3, 0, 0, 0,
		time.UTC,
	)

	initialBalance := int64(30_000_000)

	account := bson.M{
		"_id":           accountID,
		"account_no":    "1001001001",
		"customer_id":   customerID,
		"username":      "repayment-e2e",
		"email":         "repayment-e2e@example.test",
		"password_hash": "test-only",
		"type":          "payment",
		"role":          "user",
		"currency":      "VND",

		"balance": bson.M{
			"current": initialBalance,
		},

		"status":  "active",
		"version": int64(1),
		"financial_id": financialRecordID(
			accountID.Hex(),
		),
		"financial_version": int64(0),
		"opened_at":         at,
		"created_at":        at,
		"updated_at":        at,
	}

	_, err = db.Collection("accounts").InsertOne(
		ctx, account,
	)
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}

	// KYC lives in customers, not accounts. Match the Banking Write rule.
	_, err = db.Collection("customers").InsertOne(ctx, bson.M{
		"_id":           customerID,
		"customer_code": "REPAY-E2E-001",
		"profile": bson.M{
			"full_name": "Repayment E2E", "date_of_birth": nil,
			"phone": nil, "email": "repayment-e2e@example.test",
			"address": bson.M{"line": "test", "province_code": "79"},
		},
		"identity": bson.M{
			"type": nil, "number": nil, "issued_date": nil,
			"issued_place": nil, "permanent_address": nil,
		},
		"verified_kyc_session_id": nil,
		"kyc_status":              "verified",
		"credit_profile": bson.M{
			"score": int64(0), "bad_debt": false, "updated_at": at,
		},
		"status": "active", "created_at": at, "updated_at": at,
	})
	if err != nil {
		t.Fatalf("seed verified KYC customer: %v", err)
	}

	// Tao khoan vay gia lap da duoc giai ngan.
	loanID := uuid.NewString()

	app, err := loan.NewDraftApplication(
		loanID,
		"LA-"+loanID[:8],
		accountID.Hex(),
		"personal",
		accountID.Hex(),
		12_000_000,
		6,
	)
	if err != nil {
		t.Fatal(err)
	}

	app.Status = loan.ApplicationStatusApproved
	app.ApprovedAt = &at

	if err := app.RestoreLegacyDisbursement(
		at, 1600,
	); err != nil {
		t.Fatal(err)
	}

	// Tao mot Banking Event theo dung EMI.
	paymentRecord := func(
		i int,
		before int64,
	) journal.Record {
		item := app.RepaymentSchedule[i]

		y, m, d := item.DueDate.UTC().Date()

		paidAt := time.Date(
			y, m, d, 10, 0, 0, 0,
			timeutil.VietnamLocation,
		)

		txID := journal.LoanInstallmentTransactionID(
			loanID, i+1,
		)

		payment := &journal.LoanInstallmentPayment{
			PaymentID: journal.EventID(
				txID, "payment",
			),
			TransactionID: txID,
			LoanID:        loanID,
			CustomerID:    accountID.Hex(),
			AccountID:     accountID.Hex(),
			InstallmentNo: i + 1,
			DueDate:       item.DueDate,
			PaidAt:        paidAt,

			PrincipalAmount: item.PrincipalAmount,
			InterestAmount:  item.InterestAmount,
			Amount:          item.TotalPayment,

			OutstandingPrincipalAfter: item.RemainingBalance,
		}

		after := before - item.TotalPayment

		return journal.Record{
			ID: journal.EventID(
				txID,
				journal.LoanInstallmentPaid,
			),
			Type:          journal.LoanInstallmentPaid,
			At:            paidAt,
			AccountID:     accountID.Hex(),
			LoanID:        loanID,
			BalanceBefore: &before,
			BalanceAfter:  &after,
			LoanPayment:   payment,
		}
	}

	// Truong hop Bank Projector chay truoc:
	// Loan chua duoc tao trong MongoDB.
	first := paymentRecord(0, initialBalance)

	if err := projection.Apply(
		ctx, first, 0,
	); err == nil {
		t.Fatal("payment without loan should fail")
	}

	// Projection that bai thi checkpoint khong tien.
	if _, found, err := projection.Position(ctx); err != nil ||
		found {
		t.Fatalf(
			"advanced checkpoint on failed projection: %v",
			err,
		)
	}

	// Legacy loan already exists without EMI and outstanding-principal fields.
	// Backfill must enrich it instead of treating setOnInsert as success.
	loanOID := repaymentObjectID("loan", loanID)
	if _, err := db.Collection("loans").InsertOne(ctx, bson.M{
		"_id": loanOID, "loan_no": loanID,
		"principal": app.RequestedAmount, "status": "active",
	}); err != nil {
		t.Fatalf("seed legacy loan: %v", err)
	}

	// Loan Projector tao khoan vay.
	if err := NewLoanReadModel(
		client, dbName,
	).Save(ctx, app, 1); err != nil {
		t.Fatalf(
			"project disbursed loan: %v",
			err,
		)
	}

	// The same customer may use another owned payment account for a quote.
	otherAccountID := bson.NewObjectID()
	otherAccount := bson.M{}
	for k, v := range account {
		otherAccount[k] = v
	}
	otherAccount["_id"] = otherAccountID
	otherAccount["account_no"] = "1001001002"
	otherAccount["username"] = "repayment-e2e-second"
	otherAccount["email"] = "repayment-e2e-second@example.test"
	otherAccount["financial_id"] = financialRecordID(otherAccountID.Hex())
	if _, err := db.Collection("accounts").InsertOne(ctx, otherAccount); err != nil {
		t.Fatalf("seed second owned account: %v", err)
	}
	reader := NewLoanRepaymentQuery(client, dbName)
	balance, err := reader.ReadAvailableBalance(ctx, loanID, accountID.Hex(), otherAccountID.Hex())
	if err != nil || balance != initialBalance {
		t.Fatalf("other owned account rejected: balance=%d err=%v", balance, err)
	}

	// An unverified customer cannot obtain a payable quote.
	if _, err := db.Collection("customers").UpdateOne(ctx,
		bson.M{"_id": customerID}, bson.M{"$set": bson.M{"kyc_status": "not_started"}},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadAvailableBalance(ctx, loanID, accountID.Hex(), otherAccountID.Hex()); !errors.Is(err, loanqueries.ErrInvalidPaymentAccount) {
		t.Fatalf("unverified customer must not receive repayment quote: %v", err)
	}
	if _, err := db.Collection("customers").UpdateOne(ctx,
		bson.M{"_id": customerID}, bson.M{"$set": bson.M{"kyc_status": "verified"}},
	); err != nil {
		t.Fatal(err)
	}

	// Bank Projector retry va xu ly du 6 ky.
	before := initialBalance

	for i, installment := range app.RepaymentSchedule {
		record := paymentRecord(i, before)

		err := projection.Apply(
			ctx,
			record,
			uint64(i),
		)
		if err != nil {
			t.Fatalf(
				"projection installment %d: %v",
				i+1, err,
			)
		}

		before -= installment.TotalPayment

		if i == 1 {
			// Simulate interrupted migration after two paid installments.
			if _, err := db.Collection("loans").UpdateOne(ctx,
				bson.M{"_id": loanOID},
				bson.M{"$unset": bson.M{"repayment_schedule": ""}},
			); err != nil {
				t.Fatal(err)
			}
			if err := NewLoanReadModel(client, dbName).SaveLoanContract(ctx, app); err != nil {
				t.Fatalf("backfill partially repaid legacy loan: %v", err)
			}
			var halfway struct {
				Outstanding int64                     `bson:"outstanding_principal"`
				Schedule    []repaymentInstallmentRow `bson:"repayment_schedule"`
			}
			if err := db.Collection("loans").FindOne(ctx,
				bson.M{"_id": loanOID}).Decode(&halfway); err != nil {
				t.Fatal(err)
			}
			if halfway.Outstanding != app.RepaymentSchedule[1].RemainingBalance ||
				len(halfway.Schedule) != app.TermMonths ||
				halfway.Schedule[0].Status != "paid" ||
				halfway.Schedule[1].Status != "paid" ||
				halfway.Schedule[2].Status != "upcoming" {
				t.Fatalf("backfill reset partial repayments: %+v", halfway)
			}
		}

		// Replay event trung khong duoc tru lan hai.
		if err := projection.Apply(
			ctx,
			record,
			uint64(i),
		); err != nil {
			t.Fatalf(
				"duplicate replay: %v",
				err,
			)
		}
	}

	// Simulate an old partially migrated contract missing EMI after 6
	// recorded payments. Backfill must rebuild the PAID states, not reset
	// outstanding principal or settlement.
	if _, err := db.Collection("loans").UpdateOne(ctx,
		bson.M{"_id": loanOID}, bson.M{"$unset": bson.M{"repayment_schedule": ""}},
	); err != nil {
		t.Fatal(err)
	}

	// Replay giai ngan khong duoc reset
	// du no da tat toan.
	if err := NewLoanReadModel(
		client, dbName,
	).Save(ctx, app, 2); err != nil {
		t.Fatalf(
			"reproject disbursed loan: %v",
			err,
		)
	}

	var actualLoan struct {
		Status      string `bson:"status"`
		Outstanding int64  `bson:"outstanding_principal"`
	}

	err = db.Collection("loans").FindOne(
		ctx,
		bson.M{
			"_id": repaymentObjectID(
				"loan", loanID,
			),
		},
	).Decode(&actualLoan)

	if err != nil ||
		actualLoan.Status != "completed" ||
		actualLoan.Outstanding != 0 {
		t.Fatalf(
			"loan not settled: %+v, err=%v",
			actualLoan, err,
		)
	}
	var restored struct {
		Schedule []repaymentInstallmentRow `bson:"repayment_schedule"`
	}
	if err := db.Collection("loans").FindOne(ctx, bson.M{"_id": loanOID}).Decode(&restored); err != nil {
		t.Fatal(err)
	}
	if len(restored.Schedule) != app.TermMonths {
		t.Fatalf("backfill did not restore EMI: %d", len(restored.Schedule))
	}
	for _, item := range restored.Schedule {
		if item.Status != "paid" {
			t.Fatalf("backfill reset paid installment %d", item.InstallmentNo)
		}
	}

	// Moi ky phai co dung mot document
	// o tung collection lien quan.
	for _, coll := range []string{
		"loan_payments",
		"transactions",
		"ledger_entries",
		"account_transaction_feed",
	} {
		count, err := db.Collection(coll).
			CountDocuments(ctx, bson.M{})

		if err != nil ||
			count != int64(app.TermMonths) {
			t.Fatalf(
				"%s count=%d err=%v",
				coll, count, err,
			)
		}
	}

	var after struct {
		Balance struct {
			Current int64 `bson:"current"`
		} `bson:"balance"`
	}

	err = db.Collection("accounts").FindOne(
		ctx,
		bson.M{"_id": accountID},
	).Decode(&after)
	if err != nil {
		t.Fatal(err)
	}

	if after.Balance.Current != before {
		t.Fatalf(
			"balance=%d, want=%d",
			after.Balance.Current,
			before,
		)
	}

	pos, found, err := projection.Position(ctx)
	if err != nil ||
		!found ||
		pos != uint64(app.TermMonths-1) {
		t.Fatalf(
			"checkpoint=%d found=%v err=%v",
			pos, found, err,
		)
	}

	// Interrupted checkpoint-only replay must never roll the account balance
	// backward to the first installment's historical BalanceAfter.
	if err := projection.Reset(ctx); err != nil {
		t.Fatalf("reset checkpoint for replay: %v", err)
	}
	if err := projection.Apply(ctx, first, 0); err != nil {
		t.Fatalf("replay old payment: %v", err)
	}
	var replayBalance struct {
		Balance struct {
			Current int64 `bson:"current"`
		} `bson:"balance"`
	}
	if err := db.Collection("accounts").FindOne(ctx, bson.M{"_id": accountID}).Decode(&replayBalance); err != nil {
		t.Fatal(err)
	}
	if replayBalance.Balance.Current != before {
		t.Fatalf("interrupted replay rolled balance backward: %d want %d", replayBalance.Balance.Current, before)
	}

	// Xac minh Query doc duoc du lieu that.
	q := NewLoanRepaymentQuery(client, dbName)

	schedule, err := q.ReadSchedule(
		ctx, loanID, accountID.Hex(),
	)
	if err != nil ||
		schedule.Status != "completed" ||
		schedule.OutstandingPrincipal != 0 {
		t.Fatalf(
			"read schedule: %+v err=%v",
			schedule, err,
		)
	}

	payments, err := q.ReadPayments(
		ctx, loanID, accountID.Hex(),
	)
	if err != nil ||
		len(payments) != app.TermMonths {
		t.Fatalf(
			"read payments=%d err=%v",
			len(payments), err,
		)
	}

	if payments[0].InstallmentNo != app.TermMonths {
		t.Fatalf(
			"payment ordering=%d",
			payments[0].InstallmentNo,
		)
	}

	// A contradictory old outstanding balance cannot be silently rewritten.
	if _, err := db.Collection("loans").UpdateOne(ctx,
		bson.M{"_id": loanOID},
		bson.M{"$set": bson.M{"outstanding_principal": int64(1)}},
	); err != nil {
		t.Fatal(err)
	}
	if err := NewLoanReadModel(client, dbName).SaveLoanContract(ctx, app); err == nil {
		t.Fatal("inconsistent legacy outstanding accepted by backfill")
	}

	t.Log(fmt.Sprintf(
		"E2E OK, %d installments, final balance %d",
		app.TermMonths,
		before,
	))
}
