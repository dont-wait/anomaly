//go:build e2e

package eventstore_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	accountcommands "github.com/dont-wait/anomaly/internal/application/account/commands"
	loancommands "github.com/dont-wait/anomaly/internal/application/loan/commands"
	"github.com/dont-wait/anomaly/internal/composition"
	"github.com/dont-wait/anomaly/internal/domain"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	loan "github.com/dont-wait/anomaly/internal/domain/loan"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/dont-wait/anomaly/internal/infrastructure/auth"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	"github.com/dont-wait/anomaly/internal/infrastructure/loanadapter"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/infrastructure/otp"
	"github.com/dont-wait/anomaly/internal/infrastructure/rustfs"
	presentation "github.com/dont-wait/anomaly/internal/presentation/http"
	loanhttp "github.com/dont-wait/anomaly/internal/presentation/http/handler/loan"
	transactionhttp "github.com/dont-wait/anomaly/internal/presentation/http/handler/transaction"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// HTTP, JWT, command handlers và repositories đều dùng implementation thật.
// Chạy projection đồng bộ sau command để kiểm tra read model mà không phụ thuộc
// lịch chạy worker; bài test này không kiểm tra vòng lặp subscription/checkpoint.
func TestPersonalLoanHTTPE2E(t *testing.T) {
	t.Setenv("LOAN_OFFER_WRITES_ENABLED", "true")
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
	mongoClient, err := mongorepo.NewMongoClient(ctx, &domain.MongoConfig{MongoURI: parsed.String()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mongoClient.Disconnect(context.Background()) })
	db := mongoClient.Database("personal_loan_e2e")
	applyMigrations(t, ctx, db)
	es := startEventStore(t, ctx)
	media := startLoanMedia(t, ctx)
	reads := mongorepo.NewTransferRepository(mongoClient, db.Name())
	bank := eventstore.NewBankRepository(es, reads)
	repo := eventstore.NewLoanRepository(es)
	read := mongorepo.NewLoanReadModel(mongoClient, db.Name())
	if err := read.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = db.Collection("loan_packages").InsertOne(ctx, bson.M{
		"package_code": "PERSONAL_UNSECURED_SALARY", "name": "Vay tín chấp theo lương",
		"status": "ACTIVE", "min_amount": int64(1_000_000), "max_amount": int64(500_000_000),
		"term_options": bson.A{bson.M{"months": 6}, bson.M{"months": 12}, bson.M{"months": 24}},
	})
	if err != nil {
		t.Fatal(err)
	}
	catalog := mongorepo.NewLoanPackageCatalog(mongoClient, db.Name())
	if err := catalog.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	evidence := loanadapter.NewEvidenceStore(mongoClient, db.Name())
	tokens := auth.NewTokenService("isolated-loan-e2e-secret", time.Hour)
	register := accountcommands.NewRegisterAccountCommandHandler(bank, bank)
	var users []*accountdomain.UserAccount
	var accessTokens []string
	for i := range 3 {
		a, err := register.Handle(ctx, accountcommands.RegisterAccountCommand{
			IdempotencyKey: uuid.NewString(), Username: fmt.Sprintf("loan-user-%d", i),
			Email: fmt.Sprintf("loan-%d@example.com", i), Password: "TestBank@123",
			CCCDNumber: fmt.Sprintf("%012d", i+1), CCCDIssuedDate: time.Now().Add(-time.Hour),
			DOB: time.Now().AddDate(-25, 0, 0),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := accountcommands.NewVerifyAccountCommandHandler(bank).Handle(ctx, accountcommands.VerifyAccountCommand{
			AccountID: a.Id, IdCardFrontUrl: "kyc/" + a.Id + "/id-card-front/front",
			IdCardBackUrl: "kyc/" + a.Id + "/id-card-back/back", LiveVideoUrl: "kyc/" + a.Id + "/live-video/live",
		}); err != nil {
			t.Fatal(err)
		}
		a, err = bank.FindByID(ctx, a.Id)
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			a.Role = accountdomain.AccountRoleAdmin
			a.Version++
			a.UpdatedAt = time.Now().UTC()
			if err := bank.Save(ctx, a); err != nil {
				t.Fatal(err)
			}
		}
		token, _, err := tokens.Issue(a.Id, a.Username, string(a.EffectiveRole()), true)
		if err != nil {
			t.Fatal(err)
		}
		users = append(users, a)
		accessTokens = append(accessTokens, token)
	}
	owner, admin, outsider := accessTokens[0], accessTokens[1], accessTokens[2]

	// Dựng lại handler/repository khi retry để không dựa vào cache của request cũ.
	serve := func() (*httptest.Server, *loanhttp.Handler) {
		h := composition.NewLoanHandler(eventstore.NewBankRepository(es, reads), eventstore.NewLoanRepository(es), read, evidence, media)
		composition.AttachLoanProducts(h, catalog)
		h.ContractOTP = demoLoanContractOTP{}
		tx := transactionhttp.NewHandler(zerolog.Nop(), bank, nil, nil, nil)
		accounts := composition.NewAccountHandler(bank, tokens, zerolog.Nop())
		srv := httptest.NewServer(presentation.NewRouter(http.NewServeMux(), accounts, nil, nil, tx, tokens, false, h))
		t.Cleanup(srv.Close)
		return srv, h
	}
	srv, handler := serve()
	api := loanE2EAPI{ctx: ctx, client: srv.Client(), baseURL: srv.URL}
	api.json(t, "GET", "/api/loans", "", nil, nil, 401)

	var expectedBalance int64
	for _, tt := range []struct {
		name             string
		history          loan.RepaymentHistoryStatus
		wantStatus       loan.ApplicationStatus
		completeEvidence bool
		reconcile        bool
	}{
		{"complete evidence then auto approve", loan.HistoryOnTime, loan.ApplicationStatusApproved, true, false},
		{"manual approval", loan.HistoryLate, loan.ApplicationStatusUnderReview, false, false},
		{"automatic rejection", loan.HistoryOverdue, loan.ApplicationStatusRejected, false, false},
		{"recover credited loan", loan.HistoryOnTime, loan.ApplicationStatusApproved, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			create := map[string]any{
				"loanPackageId": "PERSONAL_UNSECURED_SALARY", "disbursementAccountId": users[0].Id,
				"requestedAmount": int64(10_000_000), "termMonths": 12,
			}
			headers := map[string]string{"Idempotency-Key": uuid.NewString()}
			a := decodeLoanE2E[loan.Application](t, api.json(t, "POST", "/api/loans", owner, create, headers, 201))
			base := "/api/loans/" + a.Id
			again := decodeLoanE2E[loan.Application](t, api.json(t, "POST", "/api/loans", owner, create, headers, 201))
			if a.Id != again.Id || a.Status != loan.ApplicationStatusDraft {
				t.Fatal("create retry must return the same draft")
			}
			api.json(t, "GET", base, outsider, nil, nil, 404)
			api.json(t, "POST", base+"/review", owner, nil, nil, 403)
			api.json(t, "POST", base+"/disburse", owner, nil, nil, 403)
			api.json(t, "POST", base+"/disburse", admin, nil, nil, 422)
			api.json(t, "POST", base+"/submit", owner, nil, nil, 422)

			var documents []loan.ApplicationDocument
			for _, kind := range []string{"IDENTITY_FRONT", "IDENTITY_BACK", "VNEID_PERSONAL", "VNEID_RESIDENCE", "VSSID_PARTICIPATION", "VSSID_DETAIL", "SALARY_STATEMENT_3_MONTHS"} {
				documents = append(documents, api.upload(t, base+"/documents", owner, kind))
			}
			download := base + "/documents/" + documents[0].DocumentId
			if body := api.json(t, "GET", download, owner, nil, nil, 200); !bytes.Equal(body, loanE2EPDF) {
				t.Fatal("downloaded document differs from uploaded bytes")
			}
			api.json(t, "GET", download, outsider, nil, nil, 404)
			submitted := decodeLoanE2E[loan.Application](t, api.json(t, "POST", base+"/submit", owner, nil, nil, 200))
			if submitted.Status != loan.ApplicationStatusSubmitted {
				t.Fatal("submit did not persist submitted status")
			}
			api.json(t, "POST", base+"/review", admin, nil, nil, 200)
			e := map[string]any{
				"monthlySalaries": [3]int64{20_000_000, 20_000_000, 20_000_000}, "regionCode": "KV01",
				"existingMonthlyDebt": 0, "insuranceMonths": 24, "repaymentHistory": tt.history,
				"insuranceVerified": true, "historyVerified": true, "financialDataVerified": true, "documentsVerified": true,
				"sources": []string{"salary-statement", "insurance-record", "repayment-record"},
			}
			if tt.completeEvidence {
				for _, flag := range []string{"financialDataVerified", "documentsVerified", "insuranceVerified", "historyVerified"} {
					e[flag] = false
					api.json(t, "POST", base+"/evidence", admin, e, nil, 201)
					_, before, err := repo.FindByIDWithRevision(ctx, a.Id)
					if err != nil {
						t.Fatal(err)
					}
					api.json(t, "POST", base+"/assess", admin, nil, nil, 422)
					unchanged, after, err := repo.FindByIDWithRevision(ctx, a.Id)
					if err != nil || before != after || unchanged.AssessedAt != nil || unchanged.Status != loan.ApplicationStatusUnderReview {
						t.Fatalf("incomplete assessment changed canonical state: revision %d -> %d, err=%v", before, after, err)
					}
					e[flag] = true
				}
			}
			api.json(t, "POST", base+"/evidence", admin, e, nil, 201)
			quote := decodeLoanE2E[struct {
				AnnualRateBps int64                        `json:"annualRateBps"`
				Official      bool                         `json:"official"`
				Schedule      []loan.RepaymentScheduleItem `json:"provisionalSchedule"`
			}](t, api.json(t, "GET", base+"/quote", owner, nil, nil, 200))
			if quote.Official || quote.AnnualRateBps <= 0 || len(quote.Schedule) != 12 {
				t.Fatalf("invalid provisional quote: %+v", quote)
			}
			api.json(t, "POST", base+"/assess", owner, nil, nil, 403)
			assessed := decodeLoanE2E[loan.Application](t, api.json(t, "POST", base+"/assess", admin, nil, nil, 200))
			if assessed.Status != tt.wantStatus || assessed.AssessedAt == nil || assessed.AssessedBy != users[1].Id {
				t.Fatalf("unexpected assessment: %+v", assessed)
			}
			api.json(t, "POST", base+"/evidence", admin, e, nil, 409)
			// Hồ sơ đã quyết định bị chặn bởi status (422); manual_review đã
			// assessed nhưng còn under_review bị chặn bởi assessment cũ (409).
			reassessStatus := 422
			if tt.wantStatus == loan.ApplicationStatusUnderReview {
				reassessStatus = 409
			}
			api.json(t, "POST", base+"/assess", admin, nil, nil, reassessStatus)
			if tt.wantStatus == loan.ApplicationStatusUnderReview {
				approved := decodeLoanE2E[loan.Application](t, api.json(t, "POST", base+"/decision", admin,
					map[string]string{"decision": "approved", "reason": "Verified late repayment was resolved"}, nil, 200))
				if approved.ManualReviewerId != users[1].Id || approved.Status != loan.ApplicationStatusApproved {
					t.Fatal("manual approval was not persisted")
				}
			}
			if tt.wantStatus == loan.ApplicationStatusRejected {
				api.json(t, "POST", base+"/disburse", admin, nil, nil, 422)
				assertBankBalance(t, ctx, bank, users[0].Id, expectedBalance)
				for _, record := range readBank(t, ctx, es) {
					if record.Type == journal.LoanDisbursementCredited && record.LoanID == a.Id {
						t.Fatal("rejected loan must not have a banking credit")
					}
				}
				return
			}

			// New credit must fail closed until owner confirms offer via OTP.
			api.json(t, "POST", base+"/disburse", admin, nil, nil, 422)
			api.json(t, "POST", base+"/offer/issue", owner, nil, nil, 403)
			issued := decodeLoanE2E[loan.LoanOffer](t, api.json(t, "POST", base+"/offer/issue", admin, nil, nil, 200))
			if issued.Status != loan.LoanOfferIssued || issued.AnnualRateBps <= 0 {
				t.Fatal("offer not issued")
			}
			api.json(t, "GET", base+"/offer", outsider, nil, nil, 404)
			api.json(t, "POST", base+"/offer/accept", outsider, nil, nil, 404)
			api.json(t, "POST", base+"/offer/accept", owner, nil, nil, 200)
			api.json(t, "POST", base+"/disburse", admin, nil, nil, 422)
			api.json(t, "POST", base+"/contract/otp", owner, nil, nil, 200)
			api.json(t, "POST", base+"/contract/confirm", owner, map[string]any{"otp": "bad", "acceptTerms": true}, nil, 400)
			confirmed := decodeLoanE2E[loan.LoanOffer](t, api.json(t, "POST", base+"/contract/confirm", owner, map[string]any{"otp": "123456", "acceptTerms": true}, nil, 200))
			if confirmed.Status != loan.LoanOfferConfirmed || confirmed.ConfirmedAt == nil {
				t.Fatal("offer not confirmed")
			}

			if tt.reconcile {
				// Chỉ tiêm lỗi ở lần ghi LoanDisbursed; banking credit vẫn ghi thật.
				// Đây là cửa sổ lỗi giữa hai stream mà reconciler phải khôi phục.
				handler.Disburse.Repo = failLoanFinalization{LoanRepository: repo}
				api.json(t, "POST", base+"/disburse", admin, nil, nil, 503)
				handler.Disburse.Repo = repo
				assertBankBalance(t, ctx, eventstore.NewBankRepository(es, reads), users[0].Id, expectedBalance+10_000_000)
				pending, err := repo.FindByID(ctx, a.Id)
				if err != nil || pending.Status != loan.ApplicationStatusApproved || pending.DisbursementPreparedAt == nil {
					t.Fatalf("expected prepared loan awaiting finalization: %v", err)
				}
				credits, err := eventstore.NewBankRepository(es, reads).ListCreditedLoans(ctx)
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, credit := range credits {
					if credit.LoanID != a.Id {
						continue
					}
					found = true
					reconciler := &loancommands.ReconcileCreditedLoanCommandHandler{Repo: eventstore.NewLoanRepository(es), Evidence: evidence}
					for range 2 {
						if err := reconciler.Handle(ctx, loancommands.ReconcileCreditedLoanCommand{
							LoanID: credit.LoanID, AccountID: credit.AccountID, Amount: credit.Amount, CreditedAt: credit.CreditedAt,
						}); err != nil {
							t.Fatal(err)
						}
					}
				}
				if !found {
					t.Fatal("credited loan missing from canonical banking stream")
				}
			}
			paid := decodeLoanE2E[loan.Application](t, api.json(t, "POST", base+"/disburse", admin, nil, nil, 200))
			expectedBalance += 10_000_000
			if paid.Status != loan.ApplicationStatusDisbursed || paid.DisbursedAt == nil || paid.AnnualRateBps != quote.AnnualRateBps || len(paid.RepaymentSchedule) != 12 {
				t.Fatalf("invalid disbursed application: %+v", paid)
			}
			var principal int64
			for _, installment := range paid.RepaymentSchedule {
				principal += installment.PrincipalAmount
			}
			if principal != paid.RequestedAmount || paid.RepaymentSchedule[11].RemainingBalance != 0 {
				t.Fatal("repayment schedule does not repay the principal")
			}
			_, beforeRetry, err := repo.FindByIDWithRevision(ctx, a.Id)
			if err != nil {
				t.Fatal(err)
			}
			fresh, _ := serve()
			retryAPI := loanE2EAPI{ctx: ctx, client: fresh.Client(), baseURL: fresh.URL}
			retried := decodeLoanE2E[loan.Application](t, retryAPI.json(t, "POST", base+"/disburse", admin, nil, nil, 200))
			detail := decodeLoanE2E[loan.Application](t, retryAPI.json(t, "GET", base, owner, nil, nil, 200))
			if detail.Status != loan.ApplicationStatusDisbursed || detail.DisbursedAt == nil || !detail.DisbursedAt.Equal(*paid.DisbursedAt) {
				t.Fatal("loan detail did not replay the persisted disbursement")
			}
			loaded, revision, err := eventstore.NewLoanRepository(es).FindByIDWithRevision(ctx, a.Id)
			if err != nil || beforeRetry != revision || !retried.DisbursedAt.Equal(*paid.DisbursedAt) {
				t.Fatalf("retry changed loan events or disbursement date: %v", err)
			}
			assertBankBalance(t, ctx, eventstore.NewBankRepository(es, reads), users[0].Id, expectedBalance)

			projection := mongorepo.NewBankProjection(mongoClient, db.Name())
			creditCount := 0
			for i, record := range readBank(t, ctx, es) {
				if record.Type == journal.LoanDisbursementCredited && record.LoanID == a.Id {
					creditCount++
				}
				// Giao lại event không được nhân đôi số dư, ledger hay feed.
				for range 2 {
					if err := projection.Apply(ctx, record, uint64(i)); err != nil {
						t.Fatal(err)
					}
				}
			}
			if creditCount != 1 {
				t.Fatalf("banking credit events = %d, want 1", creditCount)
			}
			financialID, err := bson.ObjectIDFromHex(journal.FinancialAccountID(users[0].Id))
			if err != nil {
				t.Fatal(err)
			}
			assertProjectedBalance(t, ctx, db, financialID, expectedBalance)
			for _, collection := range []string{"ledger_entries", "account_transaction_feed"} {
				count, err := db.Collection(collection).CountDocuments(ctx, bson.M{"transaction_id": a.Id})
				if err != nil || count != 1 {
					t.Fatalf("%s: count=%d, err=%v", collection, count, err)
				}
			}
			page := decodeLoanE2E[struct {
				Items []struct {
					ID        string `json:"id"`
					Kind      string `json:"kind"`
					Direction string `json:"direction"`
					Amount    int64  `json:"amount"`
				} `json:"items"`
			}](t, api.json(t, "GET", "/api/transactions", owner, nil, nil, 200))
			found := false
			for _, item := range page.Items {
				if item.ID == a.Id && item.Kind == "loan_disbursement" && item.Direction == "in" && item.Amount == paid.RequestedAmount {
					found = true
				}
			}
			if !found {
				t.Fatal("disbursement missing from transaction history API")
			}
			if err := read.Save(ctx, loaded, revision); err != nil {
				t.Fatal(err)
			}
			list := decodeLoanE2E[struct {
				Items []loan.Application `json:"items"`
			}](t,
				api.json(t, "GET", "/api/loans", owner, nil, nil, 200))
			found = false
			for _, item := range list.Items {
				if item.Id == a.Id && item.Status == loan.ApplicationStatusDisbursed {
					found = true
				}
			}
			if !found {
				t.Fatal("disbursed application missing from loan read model API")
			}
			// Xóa object thật để kiểm tra sentinel RustFS qua HTTP (404, không 503).
			if err := media.Delete(ctx, documents[0].StorageKey); err != nil {
				t.Fatal(err)
			}
			api.json(t, "GET", download, owner, nil, nil, 404)
		})
	}

	// Hurl là dependency tùy chọn: make test-loan-e2e vẫn chỉ yêu cầu Go/Docker.
	// Target test-loan-hurl-e2e bật suite cùng fixture và API thật ở trên.
	if os.Getenv("LOAN_HURL_E2E") == "1" {
		t.Run("hurl", func(t *testing.T) {
			runLoanHurlE2E(t, ctx, srv.URL, es, mongoClient, db.Name())
		})
	}
}

type failLoanFinalization struct{ *eventstore.LoanRepository }

func (r failLoanFinalization) Append(ctx context.Context, id string, revision uint64, eventType string, payload any) error {
	if eventType == "LoanDisbursed" {
		return errors.New("simulated EventStoreDB append failed")
	}
	return r.LoanRepository.Append(ctx, id, revision, eventType, payload)
}

func startLoanMedia(t *testing.T, ctx context.Context) *rustfs.MediaRepository {
	t.Helper()
	// Cùng image và credentials mặc định với Compose, nhưng không dùng volume
	// hoặc bucket của môi trường dev. Testcontainers dọn toàn bộ sau bài test.
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "rustfs/rustfs:latest", ExposedPorts: []string{"9000/tcp"},
			Env:        map[string]string{"RUSTFS_ACCESS_KEY": "rustfsadmin", "RUSTFS_SECRET_KEY": "rustfsadmin"},
			WaitingFor: wait.ForHTTP("/health").WithPort("9000/tcp").WithStartupTimeout(90 * time.Second),
		}, Started: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	testcontainers.CleanupContainer(t, c)
	endpoint, err := c.PortEndpoint(ctx, "9000/tcp", "http")
	if err != nil {
		t.Fatal(err)
	}
	client := rustfs.NewClient(&domain.RustFSConfig{
		Endpoint: endpoint, AccessKey: "rustfsadmin", SecretKey: "rustfsadmin", Region: "us-east-1",
	})
	const bucket = "loan-e2e"
	if err := rustfs.EnsureBucket(ctx, client, bucket); err != nil {
		t.Fatal(err)
	}
	return rustfs.NewMediaRepository(client, bucket)
}

type loanE2EAPI struct {
	ctx     context.Context
	client  *http.Client
	baseURL string
}

func (a loanE2EAPI) json(t *testing.T, method, path, token string, body any, headers map[string]string, want int) []byte {
	t.Helper()
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	return a.request(t, method, path, token, bytes.NewReader(data), "application/json", headers, want)
}

func (a loanE2EAPI) request(t *testing.T, method, path, token string, body io.Reader, contentType string, headers map[string]string, want int) []byte {
	t.Helper()
	req, err := http.NewRequestWithContext(a.ctx, method, a.baseURL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", contentType)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	res, err := a.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != want {
		t.Fatalf("%s %s: status=%d, want=%d; body=%s", method, path, res.StatusCode, want, data)
	}
	return data
}

var loanE2EPDF = []byte("%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\n%%EOF\n")

func (a loanE2EAPI) upload(t *testing.T, path, token, kind string) loan.ApplicationDocument {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := w.WriteField("type", kind); err != nil {
		t.Fatal(err)
	}
	file, err := w.CreateFormFile("file", "evidence.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(loanE2EPDF); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return decodeLoanE2E[loan.ApplicationDocument](t, a.request(t, "POST", path, token, &body, w.FormDataContentType(), nil, 201))
}

func decodeLoanE2E[T any](t *testing.T, body []byte) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, body)
	}
	return value
}

// Challenge used only by the isolated E2E HTTP server; production uses Redis+SMTP.
type demoLoanContractOTP struct{}

func (demoLoanContractOTP) Issue(_ context.Context, _, _ string) (otp.TransferOTPInfo, error) {
	return otp.TransferOTPInfo{Channel: "email", ExpiresAt: time.Now().Add(time.Minute)}, nil
}

func (demoLoanContractOTP) Verify(_ context.Context, _, code string) (int, error) {
	if code != "123456" {
		return 2, otp.ErrTransferOTPInvalid
	}
	return 3, nil
}
func (demoLoanContractOTP) Consume(_ context.Context, _ string) error { return nil }
