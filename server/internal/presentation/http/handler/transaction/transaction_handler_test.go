package transaction

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"go.mongodb.org/mongo-driver/v2/bson"

	infraauth "github.com/dont-wait/anomaly/internal/infrastructure/auth"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
)

type transferRepositoryFake struct {
	existing       mongorepo.TransferDocument
	lookupErr      error
	source         mongorepo.TransferAccount
	lookupCalls    int
	lookupKey      string
	lookupOwner    string
	sourceCalls    int
	recipientCalls int
	createCalls    int
}

func (f *transferRepositoryFake) FindOwnedAccount(context.Context, string) (mongorepo.TransferAccount, error) {
	f.sourceCalls++
	return f.source, nil
}

func (f *transferRepositoryFake) FindRecipient(context.Context, string) (mongorepo.TransferAccount, error) {
	f.recipientCalls++
	return mongorepo.TransferAccount{}, mongorepo.ErrTransactionAccountNotFound
}

func (f *transferRepositoryFake) AccountObjectID(context.Context, string) (bson.ObjectID, error) {
	return bson.ObjectID{}, nil
}

func (f *transferRepositoryFake) FindTransferByIdempotencyKey(_ context.Context, key, owner string) (mongorepo.TransferDocument, error) {
	f.lookupCalls++
	f.lookupKey, f.lookupOwner = key, owner
	return f.existing, f.lookupErr
}

func (f *transferRepositoryFake) CreatePending(context.Context, mongorepo.TransferDocument) (mongorepo.TransferDocument, bool, error) {
	f.createCalls++
	return mongorepo.TransferDocument{}, false, nil
}

func (*transferRepositoryFake) FindTransfer(context.Context, string, string) (mongorepo.TransferDocument, error) {
	return mongorepo.TransferDocument{}, nil
}

func (*transferRepositoryFake) ConfirmTransfer(context.Context, string, string) (mongorepo.TransferDocument, error) {
	return mongorepo.TransferDocument{}, nil
}

func (*transferRepositoryFake) CancelPending(context.Context, string) error { return nil }

func (*transferRepositoryFake) SetOTPMetadata(context.Context, string, time.Time, time.Time) error {
	return nil
}

func (*transferRepositoryFake) GetFeed(context.Context, bson.ObjectID, string) (mongorepo.FeedDocument, error) {
	return mongorepo.FeedDocument{}, nil
}

func (*transferRepositoryFake) ListFeed(context.Context, bson.ObjectID, bson.M, int64) ([]mongorepo.FeedDocument, error) {
	return nil, nil
}

func (*transferRepositoryFake) Summary(context.Context, bson.ObjectID, time.Time, time.Time) (int64, int64, error) {
	return 0, 0, nil
}

func createTransferRequestForTest(t *testing.T, repo Repository, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	tokens := infraauth.NewTokenService("test-secret", time.Hour)
	token, _, err := tokens.Issue("owner-id", "owner", "user", true)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	handler := NewHandler(zerolog.Nop(), repo, nil)
	secured := middleware.RequireAuth(tokens)(http.HandlerFunc(handler.CreateTransfer))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/transfers", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", key)
	secured.ServeHTTP(recorder, request)
	return recorder
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
