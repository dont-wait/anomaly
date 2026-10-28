//go:build e2e

package eventstore_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	accountcommands "github.com/dont-wait/anomaly/internal/application/account/commands"
	savingscommands "github.com/dont-wait/anomaly/internal/application/savings/commands"
	savingsqueries "github.com/dont-wait/anomaly/internal/application/savings/queries"
	"github.com/dont-wait/anomaly/internal/composition"
	"github.com/dont-wait/anomaly/internal/domain"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/domain/savings"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/dont-wait/anomaly/internal/infrastructure/auth"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	savingshttp "github.com/dont-wait/anomaly/internal/presentation/http/handler/savings"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
	"github.com/google/uuid"
	esdb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// This is the Savings HTTP -> Redis quote -> EventStoreDB -> BankProjection ->
// MongoDB -> Savings Query HTTP integration test. It deliberately drives the
// SAME projection Apply method as cmd/projector, synchronously: it does NOT
// claim to test the process's subscription/reconnect loop (see GĐ5 checklist).
// All three backends are disposable Testcontainers, never the local banking DB.
func TestSavingsHTTPProjectionFullFlow(t *testing.T) {
	t.Setenv("SAVINGS_EVENT_WRITE_ENABLED", "true")
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

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
	query := parsed.Query()
	query.Set("directConnection", "true")
	parsed.RawQuery = query.Encode()
	mongoClient, err := mongorepo.NewMongoClient(ctx, &domain.MongoConfig{MongoURI: parsed.String()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mongoClient.Disconnect(context.Background()) })
	db := mongoClient.Database("savings_full_flow_e2e")
	applyMigrations(t, ctx, db)

	rc, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "redis:7-alpine", ExposedPorts: []string{"6379/tcp"},
			WaitingFor: wait.ForLog("Ready to accept connections").WithStartupTimeout(90 * time.Second),
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

	es := startEventStore(t, ctx)
	bank := eventstore.NewBankRepository(es, mongorepo.NewTransferRepository(mongoClient, db.Name()))
	catalog := mongorepo.NewSavingsPackageCatalog(mongoClient, db.Name())
	if err := catalog.EnsureSeeded(ctx); err != nil {
		t.Fatal(err)
	}
	read := mongorepo.NewSavingsRead(mongoClient, db.Name())
	if err := read.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	projection := mongorepo.NewBankProjection(mongoClient, db.Name())
	var lastProjected int
	projectNew := func() {
		t.Helper()
		records := readBank(t, ctx, es)
		for i := lastProjected; i < len(records); i++ {
			if err := projection.Apply(ctx, records[i], uint64(i)); err != nil {
				t.Fatalf("project revision %d (%s): %v", i, records[i].Type, err)
			}
		}
		lastProjected = len(records)
	}

	register := accountcommands.NewRegisterAccountCommandHandler(bank, bank)
	createUser := func(username string, cccd string) (string, string) {
		t.Helper()
		a, err := register.Handle(ctx, accountcommands.RegisterAccountCommand{
			IdempotencyKey: uuid.NewString(), Username: username, Email: username + "@example.test",
			Password: "TestBank@123", CCCDNumber: cccd,
			CCCDIssuedDate: time.Now().Add(-48 * time.Hour), DOB: time.Now().AddDate(-25, 0, 0),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := accountcommands.NewVerifyAccountCommandHandler(bank).Handle(ctx, accountcommands.VerifyAccountCommand{
			AccountID: a.Id, IdCardFrontUrl: "kyc/" + a.Id + "/id-card-front/test",
			IdCardBackUrl: "kyc/" + a.Id + "/id-card-back/test",
			LiveVideoUrl:  "kyc/" + a.Id + "/live-video/test",
			Evidence: &accountdomain.KYCEvidence{
				MatchingStatus: "passed", LivenessStatus: "passed", ChallengePassed: true, SubjectConsistencyPassed: true,
			},
			MatchScore: 0.9, LivenessScore: 1,
		}); err != nil {
			t.Fatal(err)
		}
		return a.Id, a.Username
	}
	ownerID, ownerName := createUser("savings-full-owner", "012345678901")
	foreignID, foreignName := createUser("savings-full-foreign", "012345678902")
	const initial int64 = 5_000_000
	if err := bank.SetSeedBalance(ctx, ownerID, initial); err != nil {
		t.Fatal(err)
	}
	projectNew() // The real read layer only sees users after bank projection.

	tokens := auth.NewTokenService("savings-disposable-e2e-only", time.Hour)
	ownerToken, _, err := tokens.Issue(ownerID, ownerName, "user", true)
	if err != nil {
		t.Fatal(err)
	}
	foreignToken, _, err := tokens.Issue(foreignID, foreignName, "user", true)
	if err != nil {
		t.Fatal(err)
	}
	h := composition.NewSavingsHandler(catalog, bank, rdb, read)
	mux := http.NewServeMux()
	savingshttp.RegisterRoutes(openapi.NewRegistry(mux, false), h, tokens)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	api := loanE2EAPI{ctx: ctx, client: srv.Client(), baseURL: srv.URL}

	api.json(t, "GET", "/api/savings/contracts", "", nil, nil, http.StatusUnauthorized)
	api.json(t, "GET", "/api/savings/packages", ownerToken, nil, nil, http.StatusOK)
	quote := decodeLoanE2E[savingsqueries.QuoteResponse](t, api.json(t, "POST", "/api/savings/quotes", ownerToken,
		savingsqueries.CreateQuoteInput{PackageID: savingsqueries.StandardPackageID, SourceAccountID: ownerID, Amount: 1_000_000, TermMonths: 3}, nil, http.StatusOK))
	if quote.QuoteID == "" || quote.EstimatedInterest <= 0 {
		t.Fatalf("unexpected opening quote: %+v", quote)
	}
	assertBankBalance(t, ctx, bank, ownerID, initial) // Quote must never debit.

	openKey := uuid.NewString()
	openRequest := savingscommands.OpenInput{QuoteID: quote.QuoteID, SourceAccountID: ownerID, Confirm: true}
	openPath := "/api/savings/contracts"
	openHeaders := map[string]string{"Idempotency-Key": openKey}
	api.json(t, "POST", openPath, foreignToken, openRequest, openHeaders, http.StatusBadRequest)
	opened := decodeLoanE2E[savingscommands.OpenResponse](t, api.json(t, "POST", openPath, ownerToken, openRequest, openHeaders, http.StatusCreated))
	duplicate := decodeLoanE2E[savingscommands.OpenResponse](t, api.json(t, "POST", openPath, ownerToken, openRequest, openHeaders, http.StatusCreated))
	if opened.SavingsID == "" || opened.SavingsID != duplicate.SavingsID || opened.TransactionID != duplicate.TransactionID {
		t.Fatalf("opening retry not idempotent: first=%+v replay=%+v", opened, duplicate)
	}
	assertBankBalance(t, ctx, bank, ownerID, initial-1_000_000)
	if got := countSavingsEvents(readBank(t, ctx, es), journal.SavingsOpened); got != 1 {
		t.Fatalf("opening recorded %d debits, want exactly one", got)
	}
	projectNew()
	activeDetail := "/api/savings/contracts/" + opened.SavingsID
	api.json(t, "GET", activeDetail, foreignToken, nil, nil, http.StatusNotFound)
	detail := decodeLoanE2E[savingsqueries.SavingsContract](t, api.json(t, "GET", activeDetail, ownerToken, nil, nil, http.StatusOK))
	if detail.SavingsID != opened.SavingsID || detail.Status != savings.ContractActive || detail.Principal != 1_000_000 {
		t.Fatalf("active read model mismatch: %+v", detail)
	}
	listing := decodeLoanE2E[savingsqueries.SavingsPage](t, api.json(t, "GET", "/api/savings/contracts?status=active", ownerToken, nil, nil, http.StatusOK))
	if len(listing.Items) != 1 || listing.Items[0].SavingsID != opened.SavingsID {
		t.Fatalf("unexpected active list: %+v", listing)
	}
	// UC-11 API contract: early withdrawal and settlement receipt both fail
	// before maturity. An HTTP error must not change any account balance.
	assertSavingsE2EError(t, api.json(t, "POST", activeDetail+"/withdrawal-quotes", ownerToken,
		map[string]string{"withdrawalType": "full"}, nil, http.StatusUnprocessableEntity), "SAVINGS_NOT_MATURED")
	assertSavingsE2EError(t, api.json(t, "GET", activeDetail+"/settlement", ownerToken,
		nil, nil, http.StatusConflict), "SAVINGS_NOT_SETTLED")
	assertBankBalance(t, ctx, bank, ownerID, initial-1_000_000)

	// Test maturity without altering domain policy: append a six-month-old
	// opening to the isolated authoritative stream, as in existing GĐ3 E2E.
	actor, err := bank.FindByID(ctx, ownerID)
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().AddDate(0, -6, 0)
	oldContract, err := savings.NewContract(savings.OpenContractParams{
		ID: uuid.NewString(), SavingsNo: "SVGDFULL", CustomerID: actor.CustomerId,
		PackageID: savingsqueries.StandardPackageID, SourceAccountID: ownerID,
		Principal: 1_000_000, TermMonths: 3, AnnualRateBps: 450, OpenedAt: past,
	})
	if err != nil {
		t.Fatal(err)
	}
	before := initial - int64(1_000_000)
	after := before - oldContract.Principal
	movement := journal.SavingsMovement{
		Contract: *oldContract, ActorID: ownerID,
		OpeningQuoteID: uuid.NewString(), OpeningKey: ownerID + ":" + uuid.NewString(),
		TransactionID: journal.EventID(oldContract.ID, "deposit"),
	}
	record := journal.Record{
		ID:   journal.EventID(oldContract.ID, journal.SavingsOpened),
		Type: journal.SavingsOpened, At: past, AccountID: ownerID,
		BalanceBefore: &before, BalanceAfter: &after, Savings: &movement,
	}
	payload, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	previous := readBank(t, ctx, es)
	if len(previous) == 0 {
		t.Fatal("bank stream missing before historical fixture")
	}
	if _, err := es.AppendToStream(ctx, journal.BankStream,
		esdb.AppendToStreamOptions{StreamState: esdb.Revision(uint64(len(previous) - 1))},
		esdb.EventData{EventID: uuid.MustParse(record.ID), EventType: record.Type, ContentType: esdb.ContentTypeJson, Data: payload}); err != nil {
		t.Fatal(err)
	}
	assertBankBalance(t, ctx, eventstore.NewBankRepository(es, nil), ownerID, after)
	projectNew()
	maturedPath := "/api/savings/contracts/" + oldContract.ID
	matured := decodeLoanE2E[savingsqueries.SavingsContract](t, api.json(t, "GET", maturedPath, ownerToken, nil, nil, http.StatusOK))
	if matured.Status != savings.ContractMatured {
		t.Fatalf("historical contract must be matured: %+v", matured)
	}
	maturityWorker := mongorepo.NewSavingsMaturityWorker(mongoClient, db.Name(), bank)
	if n, err := maturityWorker.RunOnce(ctx); err != nil || n != 1 {
		t.Fatalf("first notice scan: count=%d err=%v", n, err)
	}
	if n, err := maturityWorker.RunOnce(ctx); err != nil || n != 0 {
		t.Fatalf("duplicate notice scan: count=%d err=%v", n, err)
	}
	assertBankBalance(t, ctx, bank, ownerID, after) // Scheduler never credits.
	notifications := decodeLoanE2E[savingsqueries.SavingsNotices](t, api.json(t, "GET", "/api/savings/notifications", ownerToken, nil, nil, http.StatusOK))
	if len(notifications.Items) != 1 || notifications.Items[0].SavingsID != oldContract.ID || !notifications.Items[0].CanWithdraw {
		t.Fatalf("maturity notice must offer withdrawal before settlement: %+v", notifications)
	}
	foreignNotices := decodeLoanE2E[savingsqueries.SavingsNotices](t, api.json(t,
		"GET", "/api/savings/notifications", foreignToken, nil, nil, http.StatusOK))
	if len(foreignNotices.Items) != 0 {
		t.Fatalf("maturity notification leaked to another customer: %+v", foreignNotices)
	}

	// Validate all three UC-11 routes through real JWT middleware. Error
	// responses must not modify the canonical EventStoreDB balance.
	assertSavingsE2EAuthError(t, api.json(t, "POST", maturedPath+"/withdrawal-quotes", "",
		map[string]string{"withdrawalType": "full"}, nil, http.StatusUnauthorized))
	assertSavingsE2EError(t, api.json(t, "POST", maturedPath+"/withdrawal-quotes", foreignToken,
		map[string]string{"withdrawalType": "full"}, nil, http.StatusNotFound), "SAVINGS_NOT_FOUND")
	assertSavingsE2EError(t, api.json(t, "POST", maturedPath+"/withdrawal-quotes", ownerToken,
		map[string]string{"withdrawalType": "partial"}, nil, http.StatusBadRequest), "UNSUPPORTED_WITHDRAWAL_TYPE")
	assertSavingsE2EError(t, api.json(t, "GET", maturedPath+"/settlement", ownerToken,
		nil, nil, http.StatusConflict), "SAVINGS_NOT_SETTLED")
	assertSavingsE2EError(t, api.json(t, "GET", maturedPath+"/settlement", foreignToken,
		nil, nil, http.StatusNotFound), "SAVINGS_NOT_FOUND")
	assertBankBalance(t, ctx, bank, ownerID, after)

	preview := decodeLoanE2E[savingscommands.WithdrawalPreviewResponse](t, api.json(t, "POST", maturedPath+"/withdrawal-quotes", ownerToken,
		map[string]string{"withdrawalType": "full"}, nil, http.StatusOK))
	// Independently verify day-count / 365 and HALF_UP rounding using only
	// the contract's maturity date, not the (months-later) withdrawal time.
	const annualBasis int64 = 365 * 10_000 // interest rate in basis points
	actualDays := int64(oldContract.MaturityAt.Sub(oldContract.OpenedAt) / (24 * time.Hour))
	expectedInterest := (oldContract.Principal*oldContract.AnnualRateBps*actualDays + annualBasis/2) / annualBasis
	if preview.QuoteID == "" || preview.Principal != oldContract.Principal ||
		preview.InterestAmount != expectedInterest || preview.TotalPayout != preview.Principal+expectedInterest ||
		preview.DestinationAccountID != ownerID || !preview.MaturityAt.Equal(oldContract.MaturityAt) ||
		preview.SettlementType != "at_maturity" || preview.WithdrawalType != "full" {
		t.Fatalf("invalid or post-maturity-accrued withdrawal quote: %+v; expectedInterest=%d", preview, expectedInterest)
	}
	withdrawPath := maturedPath + "/withdrawals"
	withdrawInput := savingscommands.WithdrawInput{QuoteID: preview.QuoteID, Confirm: true}
	withdrawHeaders := map[string]string{"Idempotency-Key": uuid.NewString()}
	assertSavingsE2EAuthError(t, api.json(t, "POST", withdrawPath, "", withdrawInput,
		withdrawHeaders, http.StatusUnauthorized))
	assertSavingsE2EError(t, api.json(t, "POST", withdrawPath, foreignToken, withdrawInput,
		withdrawHeaders, http.StatusNotFound), "SAVINGS_NOT_FOUND")
	assertSavingsE2EError(t, api.json(t, "POST", withdrawPath, ownerToken,
		savingscommands.WithdrawInput{QuoteID: preview.QuoteID}, withdrawHeaders, http.StatusBadRequest), "CONFIRMATION_REQUIRED")
	assertSavingsE2EError(t, api.json(t, "POST", withdrawPath, ownerToken, withdrawInput,
		nil, http.StatusBadRequest), "INVALID_IDEMPOTENCY_KEY")
	assertSavingsE2EError(t, api.json(t, "POST", withdrawPath, ownerToken, withdrawInput,
		map[string]string{"Idempotency-Key": "not-uuid-v4"}, http.StatusBadRequest), "INVALID_IDEMPOTENCY_KEY")
	assertSavingsE2EError(t, api.json(t, "POST", withdrawPath, ownerToken,
		savingscommands.WithdrawInput{QuoteID: uuid.NewString(), Confirm: true},
		map[string]string{"Idempotency-Key": uuid.NewString()}, http.StatusNotFound), "QUOTE_NOT_FOUND")
	assertBankBalance(t, ctx, bank, ownerID, after)
	if got := countSavingsEvents(readBank(t, ctx, es), journal.SavingsSettled); got != 0 {
		t.Fatalf("rejected withdrawal requests wrote %d settlement credits", got)
	}
	withdrawn := decodeLoanE2E[savingscommands.WithdrawResponse](t, api.json(t, "POST", withdrawPath, ownerToken, withdrawInput, withdrawHeaders, http.StatusOK))
	// Simulate quote eviction/expiry after successful settlement. An exact
	// same-key retry must be resolved from EventStoreDB, not Redis quote state.
	if err := rdb.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	retried := decodeLoanE2E[savingscommands.WithdrawResponse](t, api.json(t, "POST", withdrawPath, ownerToken, withdrawInput, withdrawHeaders, http.StatusOK))
	if withdrawn.TransactionID == "" || withdrawn.TransactionID != retried.TransactionID || withdrawn.TotalPayout != preview.TotalPayout {
		t.Fatalf("withdrawal retry changed receipt: first=%+v again=%+v", withdrawn, retried)
	}
	assertSavingsE2EError(t, api.json(t, "POST", withdrawPath, ownerToken,
		savingscommands.WithdrawInput{QuoteID: uuid.NewString(), Confirm: true},
		withdrawHeaders, http.StatusConflict), "IDEMPOTENCY_CONFLICT")
	assertSavingsE2EError(t, api.json(t, "POST", withdrawPath, ownerToken, withdrawInput,
		map[string]string{"Idempotency-Key": uuid.NewString()}, http.StatusConflict), "SAVINGS_ALREADY_SETTLED")
	assertSavingsE2EError(t, api.json(t, "POST", maturedPath+"/withdrawal-quotes", ownerToken,
		map[string]string{"withdrawalType": "full"}, nil, http.StatusConflict), "SAVINGS_ALREADY_SETTLED")
	assertBankBalance(t, ctx, eventstore.NewBankRepository(es, nil), ownerID, after+preview.TotalPayout)
	if got := countSavingsEvents(readBank(t, ctx, es), journal.SavingsSettled); got != 1 {
		t.Fatalf("settlement recorded %d credits, want exactly one", got)
	}
	// Recreate projection lag: EventStoreDB has settled while Mongo still says
	// active. Unset the claim, but keep the already delivered notice for history.
	if _, err := db.Collection("savings_contracts").UpdateOne(ctx, bson.M{"_id": oldContract.ID}, bson.M{"$unset": bson.M{"maturity_notice_at": ""}}); err != nil {
		t.Fatal(err)
	}
	// The worker must verify the bank stream instead of trusting stale Mongo.
	if n, err := maturityWorker.RunOnce(ctx); err != nil || n != 0 {
		t.Fatalf("worker retried a settled contract during projection lag: %d / %v", n, err)
	}
	projectNew()
	remaining := decodeLoanE2E[savingsqueries.SavingsNotices](t, api.json(t, "GET", "/api/savings/notifications", ownerToken, nil, nil, http.StatusOK))
	if len(remaining.Items) != 1 || remaining.Items[0].SavingsID != oldContract.ID || remaining.Items[0].CanWithdraw {
		t.Fatalf("settlement must retain history but revoke withdrawal action: %+v", remaining)
	}
	if n, err := db.Collection("savings_notifications").CountDocuments(ctx, bson.M{"savings_id": oldContract.ID}); err != nil || n != 1 {
		t.Fatalf("maturity notice should remain exactly once after settlement: n=%d err=%v", n, err)
	}
	finalDetail := decodeLoanE2E[savingsqueries.SavingsContract](t, api.json(t, "GET", maturedPath, ownerToken, nil, nil, http.StatusOK))
	if finalDetail.Status != savings.ContractSettled || finalDetail.Settlement == nil || finalDetail.Settlement.TotalAmount != withdrawn.TotalPayout {
		t.Fatalf("settlement not projected: %+v", finalDetail)
	}
	receipt := decodeLoanE2E[savingscommands.SettlementReceipt](t, api.json(t, "GET", maturedPath+"/settlement", ownerToken, nil, nil, http.StatusOK))
	if receipt.Settlement.TransactionID != withdrawn.TransactionID ||
		receipt.Settlement.TotalAmount != withdrawn.TotalPayout || receipt.Status != savings.ContractSettled {
		t.Fatalf("authoritative receipt mismatch: %+v", receipt)
	}
	assertSavingsE2EError(t, api.json(t, "GET", maturedPath+"/settlement", foreignToken,
		nil, nil, http.StatusNotFound), "SAVINGS_NOT_FOUND")
	history := decodeLoanE2E[savingsqueries.SavingsHistory](t, api.json(t, "GET", maturedPath+"/history", ownerToken, nil, nil, http.StatusOK))
	if len(history.Items) != 2 || history.Items[0].Direction != "DEBIT" || history.Items[1].Direction != "CREDIT" {
		t.Fatalf("ledger projection should contain one debit and one credit: %+v", history)
	}
	for _, tc := range []struct{ kind string }{{"savings_deposit"}, {"savings_withdrawal"}} {
		count, err := db.Collection("ledger_entries").CountDocuments(ctx, bson.M{"type": tc.kind})
		if err != nil || count != map[string]int64{"savings_deposit": 2, "savings_withdrawal": 1}[tc.kind] {
			t.Fatalf("ledger count %s=%d error=%v", tc.kind, count, err)
		}
	}

	// Cache outage is observable, but must not hide a committed settled record.
	if err := rdb.Close(); err != nil {
		t.Fatal(err)
	}
	degraded := decodeLoanE2E[savingsqueries.SavingsContract](t, api.json(t, "GET", maturedPath, ownerToken, nil, nil, http.StatusOK))
	if degraded.Status != savings.ContractSettled || degraded.SavingsID != oldContract.ID {
		t.Fatalf("Mongo fallback during Redis outage failed: %+v", degraded)
	}
}

// assertSavingsE2EAuthError checks the shared JWT middleware's response format.
// Unlike savings handler business errors, auth failures are wrapped in an
// httpx.ErrorResponse with an errors[] array and a specific middleware code.
func assertSavingsE2EAuthError(t *testing.T, body []byte) {
	t.Helper()
	var response httpx.ErrorResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode UC-11 JWT middleware error: %v; body=%s", err, body)
	}
	if response.Status != http.StatusUnauthorized || len(response.Errors) != 1 ||
		response.Errors[0].Code != httpx.ErrorCodeMissingAuthHeader || response.Errors[0].Detail == "" {
		t.Fatalf("wrong UC-11 JWT middleware error response: %+v; body=%s", response, body)
	}
}

// assertSavingsE2EError verifies the exact documented error code, not only its
// HTTP status. A 4xx response must never silently change the banking ledger.
func assertSavingsE2EError(t *testing.T, body []byte, wantCode string) {
	t.Helper()
	var response struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode UC-11 error response: %v; body=%s", err, body)
	}
	if response.Code != wantCode || response.Error == "" {
		t.Fatalf("UC-11 error code=%q message=%q, want code=%q", response.Code, response.Error, wantCode)
	}
}
