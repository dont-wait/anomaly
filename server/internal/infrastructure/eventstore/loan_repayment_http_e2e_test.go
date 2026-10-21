//go:build e2e && repayment_e2e

package eventstore_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	accountcommands "github.com/dont-wait/anomaly/internal/application/account/commands"
	repaycommands "github.com/dont-wait/anomaly/internal/application/loan/commands"
	repayqueries "github.com/dont-wait/anomaly/internal/application/loan/queries"
	"github.com/dont-wait/anomaly/internal/domain"
	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/dont-wait/anomaly/internal/infrastructure/auth"
	"github.com/dont-wait/anomaly/internal/infrastructure/cache"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	loanhttp "github.com/dont-wait/anomaly/internal/presentation/http/handler/loan"
	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
	"github.com/dont-wait/anomaly/internal/timeutil"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestLoanRepaymentHTTPE2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Isolated services. Never touches the local `anomaly` database.
	mc, err := mongodb.Run(ctx, "mongo:7.0", mongodb.WithReplicaSet("rs0"))
	if err != nil {
		t.Fatal(err)
	}
	testcontainers.CleanupContainer(t, mc)
	mongoURI, err := mc.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(mongoURI)
	if err != nil {
		t.Fatal(err)
	}
	qs := parsed.Query()
	qs.Set("directConnection", "true")
	parsed.RawQuery = qs.Encode()
	mongoClient, err := mongorepo.NewMongoClient(ctx, &domain.MongoConfig{MongoURI: parsed.String()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mongoClient.Disconnect(context.Background()) })
	db := mongoClient.Database("repayment_http_e2e")
	applyMigrations(t, ctx, db)

	es := startEventStore(t, ctx)
	rc, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "redis:7.4", ExposedPorts: []string{"6379/tcp"},
			WaitingFor: wait.ForListeningPort("6379/tcp").WithStartupTimeout(90 * time.Second),
		}, Started: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	testcontainers.CleanupContainer(t, rc)
	host, err := rc.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := rc.MappedPort(ctx, "6379/tcp")
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: fmt.Sprintf("%s:%s", host, port.Port())})
	t.Cleanup(func() { _ = rdb.Close() })
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}

	bank := eventstore.NewBankRepository(es, mongorepo.NewTransferRepository(mongoClient, db.Name()))
	register := accountcommands.NewRegisterAccountCommandHandler(bank, bank)
	owner, err := register.Handle(ctx, accountcommands.RegisterAccountCommand{
		IdempotencyKey: uuid.NewString(), Username: "repayment-http-e2e",
		Email: "repayment-http-e2e@example.test", Password: "LocalTest@123",
		CCCDNumber: "012345678999", CCCDIssuedDate: time.Now().Add(-48 * time.Hour),
		DOB: time.Now().AddDate(-25, 0, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = accountcommands.NewVerifyAccountCommandHandler(bank).Handle(ctx, accountcommands.VerifyAccountCommand{
		AccountID:      owner.Id,
		IdCardFrontUrl: "kyc/" + owner.Id + "/id-card-front/e2e",
		IdCardBackUrl:  "kyc/" + owner.Id + "/id-card-back/e2e",
		LiveVideoUrl:   "kyc/" + owner.Id + "/live-video/e2e",
	})
	if err != nil {
		t.Fatal(err)
	}
	owner, err = bank.FindByID(ctx, owner.Id)
	if err != nil {
		t.Fatal(err)
	}

	// Billing is strictly due-date-only. Use a real due date: today in VN.
	vn := timeutil.VietnamLocation
	today := time.Now().In(vn)
	if today.Day() > 28 {
		t.Skip("fixture needs an unambiguous previous-month anniversary (day <= 28)")
	}
	disbursed := time.Date(today.Year(), today.Month()-1, today.Day(), 10, 0, 0, 0, vn).UTC()
	loanID := uuid.NewString()
	loanRepo := eventstore.NewLoanRepository(es)
	makeDisbursedLoanFixture(t, ctx, loanRepo, loanID, owner.Id, disbursed)

	const principal int64 = 12_000_000
	if _, err := bank.CreditLoanDisbursement(ctx, loanID, owner.Id, principal); err != nil {
		t.Fatal(err)
	}
	bankProjection := mongorepo.NewBankProjection(mongoClient, db.Name())
	beforeRecords := readBank(t, ctx, es)
	for i, record := range beforeRecords {
		if err := bankProjection.Apply(ctx, record, uint64(i)); err != nil {
			t.Fatalf("project initial event %d (%s): %v", i, record.Type, err)
		}
	}
	app, revision, err := loanRepo.FindByIDWithRevision(ctx, loanID)
	if err != nil || app == nil || app.Status != loandomain.ApplicationStatusDisbursed {
		t.Fatalf("canonical loan: %+v %v", app, err)
	}
	loanRead := mongorepo.NewLoanReadModel(mongoClient, db.Name())
	if err := loanRead.Save(ctx, app, revision); err != nil {
		t.Fatal(err)
	}

	cacheAdapter := cache.NewLoanRepaymentRedis(rdb)
	querySvc := repayqueries.NewRepaymentQueryService(mongorepo.NewLoanRepaymentQuery(mongoClient, db.Name()), cacheAdapter)
	handler := &loanhttp.RepaymentHTTP{
		Queries:     querySvc,
		Commands:    &repaycommands.RepayLoanCommandHandler{Repo: loanRepo, Bank: bank},
		Idempotency: cacheAdapter,
	}
	tokens := auth.NewTokenService("test-only-http-repayment-secret", time.Hour)
	jwt, _, err := tokens.Issue(owner.Id, owner.Username, "user", true)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.RegisterRoutes(openapi.NewRegistry(mux, false), tokens)
	server := httptest.NewServer(mux)
	defer server.Close()

	call := func(method, path, token string, body any, key string, expected int) map[string]any {
		t.Helper()
		var reader io.Reader
		if body != nil {
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			reader = bytes.NewReader(raw)
		}
		req, err := http.NewRequestWithContext(ctx, method, server.URL+path, reader)
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		res, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != expected {
			t.Fatalf("%s %s: status=%d want=%d body=%s", method, path, res.StatusCode, expected, data)
		}
		var decoded map[string]any
		if len(data) > 0 && json.Unmarshal(data, &decoded) != nil {
			t.Fatalf("invalid JSON: %s", data)
		}
		return decoded
	}

	base := "/api/loans/" + loanID
	call("GET", base+"/repayment-schedule", "", nil, "", 401)
	strangerToken, _, err := tokens.Issue(bson.NewObjectID().Hex(), "stranger", "user", true)
	if err != nil {
		t.Fatal(err)
	}
	call("GET", base+"/repayment-schedule", strangerToken, nil, "", 404)
	schedule := call("GET", base+"/repayment-schedule", jwt, nil, "", 200)
	if schedule["status"] != "active" || schedule["nextInstallmentNo"] != float64(1) {
		t.Fatalf("schedule: %+v", schedule)
	}
	if len(schedule["items"].([]any)) != 6 {
		t.Fatalf("unexpected schedule: %+v", schedule)
	}
	customerOID, err := bson.ObjectIDFromHex(owner.CustomerId)
	if err != nil {
		t.Fatal(err)
	}
	// Quote and Write must reject the same unverified customer.
	if _, err := db.Collection("customers").UpdateOne(ctx,
		bson.M{"_id": customerOID},
		bson.M{"$set": bson.M{"kyc_status": "not_started"}},
	); err != nil {
		t.Fatal(err)
	}
	call("POST", base+"/repayments/quote", jwt, map[string]any{
		"installmentNo": 1, "sourceAccountId": owner.Id,
	}, "", 422)
	if _, err := db.Collection("customers").UpdateOne(ctx,
		bson.M{"_id": customerOID},
		bson.M{"$set": bson.M{"kyc_status": "verified"}},
	); err != nil {
		t.Fatal(err)
	}
	quote := call("POST", base+"/repayments/quote", jwt, map[string]any{
		"installmentNo": 1, "sourceAccountId": owner.Id,
	}, "", 200)
	quoteID, ok := quote["quoteId"].(string)
	if !ok || quoteID == "" {
		t.Fatalf("quote: %+v", quote)
	}
	actualTTL, err := rdb.TTL(ctx, "loan-repayment:quote:"+quoteID).Result()
	if err != nil || actualTTL <= 0 || actualTTL > 5*time.Minute {
		t.Fatalf("Redis quote TTL: %v err=%v", actualTTL, err)
	}
	idem := uuid.NewString()
	payRequest := map[string]any{"quoteId": quoteID, "sourceAccountId": owner.Id}
	call("POST", base+"/repayments", jwt, payRequest, "", 400)
	paid := call("POST", base+"/repayments", jwt, payRequest, idem, 200)
	if paid["installmentNo"] != float64(1) || paid["status"] != "success" {
		t.Fatalf("paid: %+v", paid)
	}
	replay := call("POST", base+"/repayments", jwt, payRequest, idem, 200)
	if replay["transactionId"] != paid["transactionId"] {
		t.Fatal("idempotency replay changed transaction")
	}
	call("POST", base+"/repayments", jwt, map[string]any{"quoteId": uuid.NewString(), "sourceAccountId": owner.Id}, idem, 409)

	payments, err := bank.ListLoanPayments(ctx, loanID)
	if err != nil || len(payments) != 1 {
		t.Fatalf("canonical payments=%d err=%v", len(payments), err)
	}
	latest := readBank(t, ctx, es)
	if len(latest) != len(beforeRecords)+1 {
		t.Fatalf("banking event count %d -> %d", len(beforeRecords), len(latest))
	}
	if latest[len(latest)-1].Type != journal.LoanInstallmentPaid {
		t.Fatal("missing LoanInstallmentPaid")
	}
	for i := len(beforeRecords); i < len(latest); i++ {
		if err := bankProjection.Apply(ctx, latest[i], uint64(i)); err != nil {
			t.Fatalf("project payment %d: %v", i, err)
		}
	}
	// The worker normally invalidates the cache after projecting.
	if err := cacheAdapter.Invalidate(ctx, owner.Id, loanID); err != nil {
		t.Fatal(err)
	}
	history := call("GET", base+"/payments", jwt, nil, "", 200)
	items, ok := history["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("history: %+v", history)
	}
	paymentID, ok := paid["paymentId"].(string)
	if !ok || len(paymentID) != 24 {
		t.Fatalf("payment ID: %v", paid["paymentId"])
	}
	detail := call("GET", base+"/payments/"+paymentID, jwt, nil, "", 200)
	if detail["paymentId"] != paymentID {
		t.Fatalf("detail: %+v", detail)
	}
	after := call("GET", base+"/repayment-schedule", jwt, nil, "", 200)
	if after["nextInstallmentNo"] != float64(2) {
		t.Fatalf("schedule after repayment: %+v", after)
	}
	assertBankBalance(t, ctx, bank, owner.Id, principal-int64(payments[0].Amount))
	for _, collection := range []string{"loan_payments", "transactions"} {
		n, err := db.Collection(collection).CountDocuments(ctx, bson.M{})
		if err != nil || n != 1 {
			t.Fatalf("%s=%d err=%v", collection, n, err)
		}
	}
	t.Logf("repayment HTTP E2E OK: loan=%s, payment=%s", loanID, paymentID)
}

func makeDisbursedLoanFixture(t *testing.T, ctx context.Context, repo *eventstore.LoanRepository, loanID, accountID string, disbursed time.Time) {
	t.Helper()
	created := disbursed.AddDate(0, 0, -8)
	app, err := loandomain.NewDraftApplication(loanID, "LA-"+loanID[:8], accountID, "personal", accountID, 12_000_000, 6)
	if err != nil {
		t.Fatal(err)
	}
	app.CreatedAt, app.UpdatedAt = created, created
	if err := repo.Create(ctx, app); err != nil {
		t.Fatal(err)
	}
	events := make([]eventstore.LoanAppendEvent, 0, 14)
	for _, kind := range []string{"IDENTITY_FRONT", "IDENTITY_BACK", "VNEID_PERSONAL", "VNEID_RESIDENCE", "VSSID_PARTICIPATION", "VSSID_DETAIL", "SALARY_STATEMENT_3_MONTHS"} {
		at := created.Add(1 * time.Hour)
		doc := loandomain.ApplicationDocument{
			DocumentId: uuid.NewString(), Type: kind, StorageKey: "test/" + kind,
			MIMEType: "application/pdf", SizeBytes: 128, SHA256: "local-e2e-only",
			VerificationStatus: "pending", UploadedAt: at,
		}
		events = append(events, eventstore.LoanAppendEvent{
			EventType: eventstore.EventLoanDocumentAdded, Payload: map[string]any{"document": doc, "updatedAt": at},
		})
	}
	submitted := created.AddDate(0, 0, 1)
	reviewed := created.AddDate(0, 0, 2)
	assessed := created.AddDate(0, 0, 3)
	approved := created.AddDate(0, 0, 4)
	prepared := created.AddDate(0, 0, 5)
	schedule, err := loandomain.CalculateRepaymentSchedule(12_000_000, 1600, 6, disbursed)
	if err != nil {
		t.Fatal(err)
	}
	events = append(events,
		eventstore.LoanAppendEvent{EventType: eventstore.EventLoanApplicationSubmitted, Payload: map[string]any{"submittedAt": submitted, "updatedAt": submitted}},
		eventstore.LoanAppendEvent{EventType: eventstore.EventLoanReviewStarted, Payload: map[string]any{"reviewStartedAt": reviewed, "reviewerId": "staff-test", "updatedAt": reviewed}},
		eventstore.LoanAppendEvent{EventType: eventstore.EventLoanApplicationAssessed, Payload: map[string]any{
			"dtiScore": 40, "insuranceScore": 25, "historyScore": 35, "totalScore": 100,
			"decision": loandomain.AssessmentAutoEligible, "reasons": []string{},
			"policyVersion": "e2e-fixture", "assessedAt": assessed, "updatedAt": assessed,
		}},
		eventstore.LoanAppendEvent{EventType: eventstore.EventLoanApplicationApproved, Payload: map[string]any{"decidedAt": approved, "updatedAt": approved}},
		eventstore.LoanAppendEvent{EventType: eventstore.EventLoanDisbursementPrepared, Payload: map[string]any{"preparedAt": prepared, "annualRateBps": 1600, "updatedAt": prepared}},
		eventstore.LoanAppendEvent{EventType: eventstore.EventLoanDisbursed, Payload: map[string]any{
			"disbursedAt": disbursed, "annualRateBps": 1600, "repaymentSchedule": schedule, "updatedAt": disbursed,
		}},
	)
	if err := repo.AppendBatch(ctx, loanID, 0, events...); err != nil {
		t.Fatal(err)
	}
}
