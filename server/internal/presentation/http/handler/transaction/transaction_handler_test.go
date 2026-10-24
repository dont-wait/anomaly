package transaction

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/dont-wait/anomaly/internal/domain/risk"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	infraauth "github.com/dont-wait/anomaly/internal/infrastructure/auth"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
)

type transferRepositoryFake struct {
	existing       mongorepo.TransferDocument
	lookupErr      error
	source         mongorepo.TransferAccount
	recipient      mongorepo.TransferAccount
	recipientErr   error
	lookupCalls    int
	lookupKey      string
	lookupOwner    string
	sourceCalls    int
	recipientCalls int
	createCalls    int
	created        mongorepo.TransferDocument
	transfer       mongorepo.TransferDocument
	feed           mongorepo.FeedDocument
	failCalls      int
}

func (f *transferRepositoryFake) FindOwnedAccount(context.Context, string) (mongorepo.TransferAccount, error) {
	f.sourceCalls++
	return f.source, nil
}

func (f *transferRepositoryFake) FindRecipient(context.Context, string) (mongorepo.TransferAccount, error) {
	f.recipientCalls++
	if f.recipientErr != nil {
		return mongorepo.TransferAccount{}, f.recipientErr
	}
	if f.recipient.ID == (bson.ObjectID{}) {
		return mongorepo.TransferAccount{}, mongorepo.ErrTransactionAccountNotFound
	}
	return f.recipient, nil
}

func (f *transferRepositoryFake) AccountObjectID(context.Context, string) (bson.ObjectID, error) {
	return bson.ObjectID{}, nil
}

func (f *transferRepositoryFake) FindTransferByIdempotencyKey(_ context.Context, key, owner string) (mongorepo.TransferDocument, error) {
	f.lookupCalls++
	f.lookupKey, f.lookupOwner = key, owner
	return f.existing, f.lookupErr
}

func (f *transferRepositoryFake) CreatePending(_ context.Context, tx mongorepo.TransferDocument) (mongorepo.TransferDocument, bool, error) {
	f.createCalls++
	f.created = tx
	return tx, false, nil
}

func (f *transferRepositoryFake) FindTransfer(context.Context, string, string) (mongorepo.TransferDocument, error) {
	return f.transfer, nil
}

func (*transferRepositoryFake) ConfirmTransfer(context.Context, string, string) (mongorepo.TransferDocument, error) {
	return mongorepo.TransferDocument{}, nil
}

func (*transferRepositoryFake) ConfirmTransferWithRisk(context.Context, string, string, *risk.Assessment) (mongorepo.TransferDocument, error) {
	return mongorepo.TransferDocument{}, nil
}

func (f *transferRepositoryFake) FailTransfer(context.Context, string, string, risk.Assessment) (mongorepo.TransferDocument, error) {
	f.failCalls++
	f.transfer.Status = "failed"
	return f.transfer, nil
}

func (*transferRepositoryFake) CancelPending(context.Context, string) error { return nil }

func (*transferRepositoryFake) SetOTPMetadata(context.Context, string, time.Time, time.Time) error {
	return nil
}

func (f *transferRepositoryFake) GetFeed(context.Context, bson.ObjectID, string) (mongorepo.FeedDocument, error) {
	return f.feed, nil
}

func (*transferRepositoryFake) ListFeed(context.Context, bson.ObjectID, bson.M, int64) ([]mongorepo.FeedDocument, error) {
	return nil, nil
}

func (*transferRepositoryFake) Summary(context.Context, bson.ObjectID, time.Time, time.Time) (int64, int64, error) {
	return 0, 0, nil
}

func createTransferRequestForTest(t *testing.T, repo Repository, key, body string) *httptest.ResponseRecorder {
	return createTransferRequestWithHeaders(t, repo, key, body, nil)
}

func createTransferRequestWithHeaders(t *testing.T, repo Repository, key, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	tokens := infraauth.NewTokenService("test-secret", time.Hour)
	token, _, err := tokens.Issue("owner-id", "owner", "user", true)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	handler := NewHandler(zerolog.Nop(), repo, nil, nil, nil)
	secured := middleware.RequireAuth(tokens)(http.HandlerFunc(handler.CreateTransfer))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/transfers", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", key)
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	secured.ServeHTTP(recorder, request)
	return recorder
}

func TestCreateTransferStoresModelMetadataFromHeaders(t *testing.T) {
	repo := &transferRepositoryFake{
		lookupErr: mongorepo.ErrTransactionAccountNotFound,
		source:    mongorepo.TransferAccount{ID: bson.NewObjectID(), AccountNo: "99999180105", Status: "active", Email: "owner@example.com"},
		recipient: mongorepo.TransferAccount{ID: bson.NewObjectID(), AccountNo: "99999180147", Name: "TRAN THI BICH"},
	}
	recorder := createTransferRequestWithHeaders(t, repo, "34297a72-9433-4adf-8958-868d5e50d197", `{"toBankCode":"ANOMALY","toAccountNo":"99999180147","amount":50000000,"note":"test"}`, map[string]string{
		"X-Device-ID": "550e8400-e29b-41d4-a716-446655440000",
		"X-Platform":  "android",
	})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", recorder.Code, recorder.Body.String())
	}
	if repo.created.DeviceID != "550e8400-e29b-41d4-a716-446655440000" || repo.created.Platform != "ANDROID" || repo.created.Channel != "mobile" {
		t.Fatalf("created metadata = device=%q platform=%q channel=%q", repo.created.DeviceID, repo.created.Platform, repo.created.Channel)
	}
}

func TestNormalizeDeviceIDRejectsCallerSuppliedNonUUID(t *testing.T) {
	if got := normalizeDeviceID("DEVICE01"); got != "" {
		t.Fatalf("normalizeDeviceID() = %q, want empty for untrusted arbitrary ID", got)
	}
	if got := normalizeDeviceID("550e8400-e29b-41d4-a716-446655440000"); got == "" {
		t.Fatal("normalizeDeviceID() rejected a valid device UUID")
	}
}

type riskDecisionFake struct {
	decision    risk.Decision
	found       bool
	finalized   int
	finalizeErr error
}

func (f *riskDecisionFake) Find(context.Context, string) (risk.Decision, bool, error) {
	return f.decision, f.found, nil
}

func (f *riskDecisionFake) Finalize(context.Context, string) error {
	f.finalized++
	return f.finalizeErr
}

func confirmRequestForTest(t *testing.T, h *Handler, transferID string) *httptest.ResponseRecorder {
	t.Helper()
	tokens := infraauth.NewTokenService("test-secret", time.Hour)
	token, _, err := tokens.Issue("owner-id", "owner", "user", true)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/transfers/"+transferID+"/confirm", strings.NewReader(`{"otp":"123456"}`))
	request.SetPathValue("transferId", transferID)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	middleware.RequireAuth(tokens)(http.HandlerFunc(h.ConfirmTransfer)).ServeHTTP(recorder, request)
	return recorder
}

func TestConfirmTransferReturnsPendingWithoutConsumingOTP(t *testing.T) {
	transferID := uuid.NewString()
	repo := &transferRepositoryFake{
		source:   mongorepo.TransferAccount{ID: bson.NewObjectID()},
		transfer: mongorepo.TransferDocument{ID: transferID, Status: "awaiting_otp"},
	}
	riskStore := &riskDecisionFake{decision: risk.Decision{TransferID: transferID, SourceEventID: journal.EventID(transferID, journal.Created), Decision: risk.DecisionPending}, found: true}
	h := NewHandler(zerolog.Nop(), repo, nil, nil, nil)
	h.SetRiskDecisionStore(riskStore)

	response := confirmRequestForTest(t, h, transferID)
	if response.Code != http.StatusAccepted {
		t.Fatalf("pending status = %d, want 202; body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "RISK_CHECK_PENDING") {
		t.Fatalf("pending response = %s, want RISK_CHECK_PENDING", response.Body.String())
	}
	if repo.failCalls != 0 || riskStore.finalized != 0 {
		t.Fatalf("pending mutated transfer: failCalls=%d finalized=%d", repo.failCalls, riskStore.finalized)
	}
}

func TestConfirmTransferBlocksAndIsIdempotentOnRedelivery(t *testing.T) {
	transferID := uuid.NewString()
	repo := &transferRepositoryFake{
		source:   mongorepo.TransferAccount{ID: bson.NewObjectID()},
		transfer: mongorepo.TransferDocument{ID: transferID, Status: "awaiting_otp"},
		feed:     mongorepo.FeedDocument{TransactionID: transferID, Direction: "OUT", Status: "failed"},
	}
	riskStore := &riskDecisionFake{decision: risk.Decision{
		TransferID: transferID, SourceEventID: journal.EventID(transferID, journal.Created), Decision: risk.DecisionBlock,
		Assessment: risk.Assessment{Decision: risk.DecisionBlock, IsAnomalous: true, RiskScore: 0.97, ModelVersion: "model-v1", EvaluatedAt: time.Now().UTC()},
	}, found: true}
	h := NewHandler(zerolog.Nop(), repo, nil, nil, nil)
	h.SetRiskDecisionStore(riskStore)

	first := confirmRequestForTest(t, h, transferID)
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), `"status":"failed"`) {
		t.Fatalf("blocked response = %d/%s, want failed transaction", first.Code, first.Body.String())
	}
	second := confirmRequestForTest(t, h, transferID)
	if second.Code != http.StatusOK || repo.failCalls != 1 {
		t.Fatalf("redelivery response = %d/%s, failCalls=%d; want one failure append", second.Code, second.Body.String(), repo.failCalls)
	}
	if riskStore.finalized != 2 {
		t.Fatalf("finalize calls = %d, want retry on terminal requests", riskStore.finalized)
	}
}

func TestTerminalConfirmRetriesRiskFinalization(t *testing.T) {
	for _, status := range []string{"success", "failed", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			id := uuid.NewString()
			repo := &transferRepositoryFake{transfer: mongorepo.TransferDocument{ID: id, Status: status},
				feed: mongorepo.FeedDocument{TransactionID: id, Status: status}}
			store := &riskDecisionFake{finalizeErr: errors.New("mongo unavailable")}
			h := NewHandler(zerolog.Nop(), repo, nil, nil, nil)
			h.SetRiskDecisionStore(store)
			if response := confirmRequestForTest(t, h, id); response.Code != http.StatusOK {
				t.Fatalf("canonical outcome lost after finalize failure: %s", response.Body.String())
			}
			store.finalizeErr = nil
			if response := confirmRequestForTest(t, h, id); response.Code != http.StatusOK {
				t.Fatalf("terminal retry failed: %s", response.Body.String())
			}
			if store.finalized != 2 || repo.failCalls != 0 {
				t.Fatalf("finalize=%d failures=%d; want cleanup retried without another append", store.finalized, repo.failCalls)
			}
		})
	}
}

type recipientContactsFake struct {
	items     []mongorepo.RecipientContact
	listLimit int64
}

func (f *recipientContactsFake) List(_ context.Context, ownerID bson.ObjectID, _ string, limit int64) ([]mongorepo.RecipientContact, error) {
	f.listLimit = limit
	items := make([]mongorepo.RecipientContact, 0)
	for _, item := range f.items {
		if item.OwnerAccountID == ownerID {
			items = append(items, item)
		}
	}
	return items, nil
}

func (f *recipientContactsFake) Add(_ context.Context, ownerID bson.ObjectID, accountNo, name, bankCode string) (bool, error) {
	for _, item := range f.items {
		if item.OwnerAccountID == ownerID && item.RecipientAccountNo == accountNo && item.BankCode == bankCode {
			return false, nil
		}
	}
	f.items = append(f.items, mongorepo.RecipientContact{OwnerAccountID: ownerID, RecipientAccountNo: accountNo, RecipientName: name, BankCode: bankCode})
	return true, nil
}

func (f *recipientContactsFake) Delete(_ context.Context, ownerID bson.ObjectID, accountNo, bankCode string) (bool, error) {
	for i, item := range f.items {
		if item.OwnerAccountID == ownerID && item.RecipientAccountNo == accountNo && item.BankCode == bankCode {
			f.items = append(f.items[:i], f.items[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func recipientContactRequestForTest(t *testing.T, h *Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	tokens := infraauth.NewTokenService("test-secret", time.Hour)
	token, _, err := tokens.Issue("owner-id", "owner", "user", true)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	if method == http.MethodDelete {
		request.SetPathValue("bankCode", "ANOMALY")
		request.SetPathValue("accountNo", "99999180147")
	}
	var handler http.HandlerFunc
	switch method {
	case http.MethodGet:
		handler = h.ListRecipientContacts
	case http.MethodPost:
		handler = h.AddRecipientContact
	case http.MethodDelete:
		handler = h.DeleteRecipientContact
	default:
		t.Fatalf("unsupported test method %s", method)
	}
	middleware.RequireAuth(tokens)(handler).ServeHTTP(recorder, request)
	return recorder
}

func TestRecipientContactEndpointsAreOwnerScopedAndIdempotent(t *testing.T) {
	ownerID := bson.NewObjectID()
	otherOwnerID := bson.NewObjectID()
	recipientID := bson.NewObjectID()
	repo := &transferRepositoryFake{
		source:    mongorepo.TransferAccount{ID: ownerID, AccountNo: "99999180105"},
		recipient: mongorepo.TransferAccount{ID: recipientID, AccountNo: "99999180147", Name: "TRAN THI BICH"},
	}
	contacts := &recipientContactsFake{items: []mongorepo.RecipientContact{{
		OwnerAccountID: otherOwnerID, RecipientAccountNo: "99999180199", RecipientName: "OTHER OWNER", BankCode: "ANOMALY",
	}}}
	h := NewHandler(zerolog.Nop(), repo, nil, nil, contacts)

	first := recipientContactRequestForTest(t, h, http.MethodPost, "/api/contacts/recipients", `{"accountNo":"99999180147","bankCode":"ANOMALY"}`)
	if first.Code != http.StatusCreated {
		t.Fatalf("first save status = %d, want %d; body = %s", first.Code, http.StatusCreated, first.Body.String())
	}
	second := recipientContactRequestForTest(t, h, http.MethodPost, "/api/contacts/recipients", `{"accountNo":"99999180147","bankCode":"ANOMALY"}`)
	if second.Code != http.StatusOK {
		t.Fatalf("duplicate save status = %d, want %d; body = %s", second.Code, http.StatusOK, second.Body.String())
	}

	list := recipientContactRequestForTest(t, h, http.MethodGet, "/api/contacts/recipients", "")
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d; body = %s", list.Code, http.StatusOK, list.Body.String())
	}
	var listed struct {
		Items []recipientContactResponse `json:"items"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listed.Items) != 1 || listed.Items[0].AccountNo != "99999180147" {
		t.Fatalf("listed contacts = %#v, want one saved recipient", listed.Items)
	}
	if contacts.listLimit != 50 {
		t.Fatalf("list limit = %d, want default limit 50", contacts.listLimit)
	}

	deleted := recipientContactRequestForTest(t, h, http.MethodDelete, "/api/contacts/recipients/ANOMALY/99999180147", "")
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d; body = %s", deleted.Code, http.StatusNoContent, deleted.Body.String())
	}
	missing := recipientContactRequestForTest(t, h, http.MethodDelete, "/api/contacts/recipients/ANOMALY/99999180147", "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("second delete status = %d, want %d; body = %s", missing.Code, http.StatusNotFound, missing.Body.String())
	}
}

func TestAddRecipientContactRejectsOwnAccount(t *testing.T) {
	ownerID := bson.NewObjectID()
	repo := &transferRepositoryFake{
		source:    mongorepo.TransferAccount{ID: ownerID, AccountNo: "99999180105"},
		recipient: mongorepo.TransferAccount{ID: ownerID, AccountNo: "99999180105", Name: "OWNER"},
	}
	h := NewHandler(zerolog.Nop(), repo, nil, nil, &recipientContactsFake{})
	recorder := recipientContactRequestForTest(t, h, http.MethodPost, "/api/contacts/recipients", `{"accountNo":"99999180105","bankCode":"ANOMALY"}`)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("self-contact status = %d, want %d; body = %s", recorder.Code, http.StatusUnprocessableEntity, recorder.Body.String())
	}
}

func existingTransfer() mongorepo.TransferDocument {
	tx := mongorepo.TransferDocument{
		ID:             "8bfbe3d4-742c-4602-b2bf-31ccf62de7ad",
		Status:         "awaiting_otp",
		Amount:         100000,
		Note:           "Chuyen tien",
		OTPExpiresAt:   time.Date(2026, 10, 7, 5, 5, 0, 0, time.UTC),
		OTPResendAt:    time.Date(2026, 10, 7, 5, 0, 30, 0, time.UTC),
		IdempotencyKey: "34297a72-9433-4adf-8958-868d5e50d197",
	}
	tx.Destination.AccountNo = "12345678901234"
	tx.Destination.Name = "Nguyen Van B"
	tx.Destination.BankCode = "ANOMALY"
	return tx
}

func TestTransferReferenceUsesFullUppercaseCompactUUID(t *testing.T) {
	id := "8bfbe3d4-742c-4602-b2bf-31ccf62de7ad"
	if got, want := transferReference(id), "FT8BFBE3D4742C4602B2BF31CCF62DE7AD"; got != want {
		t.Fatalf("transferReference() = %q, want %q", got, want)
	}
}

func TestNormalizeSearchHandlesUppercaseVietnameseD(t *testing.T) {
	if got, want := normalizeSearch("  Đặng Đình  "), "dang dinh"; got != want {
		t.Fatalf("normalizeSearch() = %q, want %q", got, want)
	}
}

func TestParseTransactionDateRange(t *testing.T) {
	minDate, maxDate := transactionDateBounds(time.Now())
	tests := []struct {
		name string
		from time.Time
		to   time.Time
		code string
	}{
		{name: "valid", from: minDate, to: maxDate},
		{name: "too old", from: minDate.Add(-time.Nanosecond), to: maxDate, code: "DATE_RANGE_TOO_OLD"},
		{name: "future", from: minDate, to: maxDate.Add(time.Nanosecond), code: "DATE_RANGE_IN_FUTURE"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := url.Values{}
			query.Set("from", test.from.Format(time.RFC3339Nano))
			query.Set("to", test.to.Format(time.RFC3339Nano))
			request := httptest.NewRequest(http.MethodGet, "/api/transactions?"+query.Encode(), nil)
			from, to, present, code, message := parseTransactionDateRange(request)

			if code != test.code {
				t.Fatalf("code = %q, want %q (%s)", code, test.code, message)
			}
			if test.code == "" && (!present || !from.Equal(test.from) || !to.Equal(test.to)) {
				t.Fatalf("range = %s - %s, want %s - %s", from, to, test.from, test.to)
			}
		})
	}
}

func TestCreateTransferMatchingRetrySkipsMutableValidationAndCreation(t *testing.T) {
	repo := &transferRepositoryFake{
		existing: existingTransfer(),
		source:   mongorepo.TransferAccount{Email: "owner@example.com", Status: "suspended"},
	}
	recorder := createTransferRequestForTest(t, repo, strings.ToUpper(repo.existing.IdempotencyKey), `{"toBankCode":" anomaly ","toAccountNo":" 12345678901234 ","amount":100000,"note":" Chuyen tien "}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", recorder.Code, recorder.Body.String())
	}
	if repo.lookupCalls != 1 || repo.sourceCalls != 1 || repo.recipientCalls != 0 || repo.createCalls != 0 {
		t.Fatalf("calls lookup/source/recipient/create = %d/%d/%d/%d, want 1/1/0/0", repo.lookupCalls, repo.sourceCalls, repo.recipientCalls, repo.createCalls)
	}
	if repo.lookupKey != repo.existing.IdempotencyKey || repo.lookupOwner != "owner-id" {
		t.Fatalf("lookup key/owner = %q/%q", repo.lookupKey, repo.lookupOwner)
	}
	var response createTransferResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.TransferID != repo.existing.ID || response.OTP.MaskedDestination != "ow***@example.com" {
		t.Fatalf("response = %#v", response)
	}
}

func TestCreateTransferConflictingRetryReturnsBeforeAccountValidation(t *testing.T) {
	repo := &transferRepositoryFake{existing: existingTransfer()}
	recorder := createTransferRequestForTest(t, repo, repo.existing.IdempotencyKey, `{"toBankCode":"ANOMALY","toAccountNo":"12345678901234","amount":200000,"note":"Chuyen tien"}`)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", recorder.Code, recorder.Body.String())
	}
	if repo.sourceCalls != 0 || repo.recipientCalls != 0 || repo.createCalls != 0 {
		t.Fatalf("calls source/recipient/create = %d/%d/%d, want 0/0/0", repo.sourceCalls, repo.recipientCalls, repo.createCalls)
	}
	var response struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("code = %q, want IDEMPOTENCY_CONFLICT", response.Code)
	}
}
