//go:build e2e

package eventstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/application/account/commands"
	"github.com/dont-wait/anomaly/internal/domain"
	maildomain "github.com/dont-wait/anomaly/internal/domain/mail"
	"github.com/dont-wait/anomaly/internal/domain/risk"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/dont-wait/anomaly/internal/infrastructure/auth"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/infrastructure/otp"
	transferhttp "github.com/dont-wait/anomaly/internal/presentation/http/handler/transaction"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type riskOTPTestMail struct{}

func (riskOTPTestMail) Send(context.Context, maildomain.MailMessage) error { return nil }

// Simulate an API crash/outage after EventStoreDB commits but before Mongo cleanup.
type unavailableRiskFinalizer struct {
	*mongorepo.RiskDecisionRepository
}

func (unavailableRiskFinalizer) Finalize(context.Context, string) error {
	return errors.New("simulated finalize outage")
}

func TestTransferRiskALLOWHTTPE2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	mc, err := mongodb.Run(ctx, "mongo:7.0", mongodb.WithReplicaSet("rs0"))
	if err != nil {
		t.Fatal(err)
	}
	testcontainers.CleanupContainer(t, mc)
	uri, err := mc.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("directConnection", "true")
	parsed.RawQuery = query.Encode()
	client, err := mongorepo.NewMongoClient(ctx, &domain.MongoConfig{MongoURI: parsed.String()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	db := client.Database("transfer_risk_http_e2e")
	applyMigrations(t, ctx, db)
	es := startEventStore(t, ctx)
	rc, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "redis:7-alpine", ExposedPorts: []string{"6379/tcp"},
			WaitingFor: wait.ForLog("Ready to accept connections").WithStartupTimeout(time.Minute),
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
	bank := eventstore.NewBankRepository(es, mongorepo.NewTransferRepository(client, db.Name()))
	register := commands.NewRegisterAccountCommandHandler(bank, bank)
	var ids []string
	for i := range 2 {
		a, err := register.Handle(ctx, commands.RegisterAccountCommand{
			IdempotencyKey: uuid.NewString(),
			Username:       fmt.Sprintf("risk-user-%d", i), Email: fmt.Sprintf("risk-%d@example.test", i), Password: "LocalTest@123",
			CCCDNumber: fmt.Sprintf("%012d", i+1), CCCDIssuedDate: time.Now().Add(-24 * time.Hour), DOB: time.Now().AddDate(-25, 0, 0),
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, a.Id)
	}
	if _, err := commands.NewVerifyAccountCommandHandler(bank).Handle(ctx, commands.VerifyAccountCommand{
		AccountID: ids[0], IdCardFrontUrl: "kyc/" + ids[0] + "/id-card-front/front",
		IdCardBackUrl: "kyc/" + ids[0] + "/id-card-back/back", LiveVideoUrl: "kyc/" + ids[0] + "/live-video/live",
	}); err != nil {
		t.Fatal(err)
	}
	if err := bank.SetSeedBalance(ctx, ids[0], 10000); err != nil {
		t.Fatal(err)
	}
	destination, err := bank.FindOwnedAccount(ctx, ids[1])
	if err != nil {
		t.Fatal(err)
	}
	store := mongorepo.NewRiskDecisionRepository(client, db.Name())
	if err := store.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	otps := otp.NewTransferOTPStore(rdb, riskOTPTestMail{})
	handler := transferhttp.NewHandler(zerolog.Nop(), bank, otps, nil, nil)
	handler.SetRiskDecisionStore(store)
	tokens := auth.NewTokenService("risk-e2e-secret", time.Hour)
	token, _, err := tokens.Issue(ids[0], "owner", "user", true)
	if err != nil {
		t.Fatal(err)
	}
	call := func(fn http.HandlerFunc, id, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/transfers/"+id+"/confirm", strings.NewReader(body)).WithContext(ctx)
		req.SetPathValue("transferId", id)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", uuid.NewString())
		res := httptest.NewRecorder()
		middleware.RequireAuth(tokens)(fn).ServeHTTP(res, req)
		return res
	}
	// Create through HTTP so issuance and TransferOTPUpdated are real, too.
	created := call(handler.CreateTransfer, "", fmt.Sprintf(`{"toBankCode":"ANOMALY","toAccountNo":%q,"amount":1000,"note":"risk e2e"}`, destination.AccountNo))
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	var intent struct {
		TransferID string `json:"transferId"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &intent); err != nil {
		t.Fatal(err)
	}
	id := intent.TransferID
	code, err := rdb.Get(ctx, "transfer:otp:"+id).Result()
	if err != nil {
		t.Fatal(err)
	}
	if res := call(handler.ConfirmTransfer, id, `{"otp":"000000"}`); res.Code != http.StatusAccepted {
		t.Fatalf("pending: %d %s", res.Code, res.Body.String())
	}
	if rdb.Get(ctx, "transfer:otp:"+id).Val() != code || rdb.Exists(ctx, "transfer:otp:attempts:"+id).Val() != 0 {
		t.Fatal("pending consumed OTP/attempt")
	}
	decision := risk.Decision{
		TransferID: id, SourceEventID: journal.EventID(id, journal.Created), Decision: risk.DecisionAllow,
		Assessment: risk.Assessment{Decision: risk.DecisionAllow, RiskScore: 0.08, ModelVersion: "e2e-v1", EvaluatedAt: time.Now().UTC()},
	}
	if err := store.Save(ctx, decision); err != nil {
		t.Fatal(err)
	}
	wrong := "000000"
	if wrong == code {
		wrong = "111111"
	}
	if res := call(handler.ConfirmTransfer, id, fmt.Sprintf(`{"otp":%q}`, wrong)); res.Code != http.StatusBadRequest {
		t.Fatalf("wrong OTP: %d %s", res.Code, res.Body.String())
	}
	assertBankBalance(t, ctx, bank, ids[0], 10000)
	assertBankBalance(t, ctx, bank, ids[1], 0)
	handler.SetRiskDecisionStore(unavailableRiskFinalizer{store})
	res := call(handler.ConfirmTransfer, id, fmt.Sprintf(`{"otp":%q}`, code))
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"status":"success"`) {
		t.Fatalf("allow: %d %s", res.Code, res.Body.String())
	}
	assertBankBalance(t, ctx, bank, ids[0], 9000)
	assertBankBalance(t, ctx, bank, ids[1], 1000)
	if rdb.Exists(ctx, "transfer:otp:"+id, "transfer:otp:verified:"+id).Val() != 0 {
		t.Fatal("committed transfer did not consume OTP")
	}
	if _, found, err := store.Find(ctx, id); err != nil || !found {
		t.Fatalf("outage fixture not active: %v/%v", found, err)
	}
	// Project the journal: cleanup must recover without needing another HTTP request.
	projection := mongorepo.NewBankProjection(client, db.Name())
	records := readBank(t, ctx, es)
	completed := 0
	for i, record := range records {
		if record.Type == journal.Completed && record.Event.TransactionID == id {
			completed++
			if record.Event.Payload.RiskAssessment == nil || record.Event.Payload.RiskAssessment.Decision != risk.DecisionAllow || record.Event.Payload.SourceBalanceAfter == nil || record.Event.Payload.DestinationBalanceAfter == nil {
				t.Fatal("completed event missing assessment or balances")
			}
		}
		if err := projection.Apply(ctx, record, uint64(i)); err != nil {
			t.Fatal(err)
		}
	}
	if completed != 1 {
		t.Fatalf("completed event count=%d", completed)
	}
	if _, found, err := store.Find(ctx, id); err != nil || found {
		t.Fatalf("projector did not finalize risk: %v/%v", found, err)
	}
	var persisted risk.Decision
	if err := db.Collection("risk_decisions").FindOne(ctx, bson.M{"transfer_id": id}).Decode(&persisted); err != nil || !persisted.Finalized {
		t.Fatalf("risk audit not retained/finalized: %+v/%v", persisted, err)
	}
	handler.SetRiskDecisionStore(store)
	if err := store.Save(ctx, decision); err != nil {
		t.Fatal(err)
	}
	if res := call(handler.ConfirmTransfer, id, `{"otp":"bad"}`); res.Code != http.StatusOK {
		t.Fatalf("retry: %d %s", res.Code, res.Body.String())
	}
	assertBankBalance(t, ctx, bank, ids[0], 9000)
	assertBankBalance(t, ctx, bank, ids[1], 1000)
	if got := len(readBank(t, ctx, es)); got != len(records) {
		t.Fatalf("retry appended %d journal records", got-len(records))
	}
}
