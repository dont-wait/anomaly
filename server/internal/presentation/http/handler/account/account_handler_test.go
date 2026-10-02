package account_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/dont-wait/anomaly/internal/composition"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/infrastructure/auth"
	handleraccount "github.com/dont-wait/anomaly/internal/presentation/http/handler/account"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
)

type memoryAccountRepository struct {
	accounts map[string]*accountdomain.UserAccount
}

func newMemoryAccountRepository() *memoryAccountRepository {
	return &memoryAccountRepository{accounts: map[string]*accountdomain.UserAccount{}}
}

func (r *memoryAccountRepository) Create(_ context.Context, acc *accountdomain.UserAccount) error {
	if _, exists := r.accounts[acc.Id]; exists {
		return accountdomain.ErrUserAlreadyExists
	}
	r.accounts[acc.Id] = acc
	return nil
}

func (r *memoryAccountRepository) Save(_ context.Context, acc *accountdomain.UserAccount) error {
	r.accounts[acc.Id] = acc
	return nil
}

func (r *memoryAccountRepository) FindByID(_ context.Context, id string) (*accountdomain.UserAccount, error) {
	return r.accounts[id], nil
}

func (r *memoryAccountRepository) FindByEmail(_ context.Context, email string) (*accountdomain.UserAccount, error) {
	for _, acc := range r.accounts {
		if acc.Email == email {
			return acc, nil
		}
	}
	return nil, nil
}

func (r *memoryAccountRepository) FindByUsername(_ context.Context, username string) (*accountdomain.UserAccount, error) {
	for _, acc := range r.accounts {
		if acc.Username == username {
			return acc, nil
		}
	}
	return nil, nil
}

func (r *memoryAccountRepository) FindByCCCDNumber(_ context.Context, cccd string) (*accountdomain.UserAccount, error) {
	for _, acc := range r.accounts {
		if acc.Customer != nil && acc.Customer.Identity.Number == cccd {
			return acc, nil
		}
	}
	return nil, nil
}

func (r *memoryAccountRepository) FindAll(context.Context) ([]*accountdomain.UserAccount, error) {
	accounts := make([]*accountdomain.UserAccount, 0, len(r.accounts))
	for _, acc := range r.accounts {
		accounts = append(accounts, acc)
	}
	return accounts, nil
}

func request(t *testing.T, method, body string, handler http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, "/", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	handler(recorder, request)
	return recorder
}

func routeRequest(t *testing.T, mux *http.ServeMux, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Authorization", "Bearer "+token)
	mux.ServeHTTP(recorder, request)
	return recorder
}

func TestAccountHandlersUseResponseEnvelope(t *testing.T) {
	repo := newMemoryAccountRepository()
	tokenService := auth.NewTokenService("test-secret", time.Hour)
	handler := composition.NewAccountHandler(
		repo,
		tokenService,
		zerolog.Nop(),
	)

	register := request(t, http.MethodPost, `{
		"username":"alice",
		"cccdNumber":"001234567890",
		"cccdIssuedDate":"2020-01-01T00:00:00Z",
		"dob":"2000-01-01T00:00:00Z",
		"email":"alice@example.com",
		"password":"password123"
	}`, handler.Register)
	if register.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want 201", register.Code)
	}
	registered := decodeSuccess[map[string]any](t, register)
	if registered.Status != http.StatusCreated || registered.Message == "" {
		t.Fatalf("register envelope = %#v, want status and message", registered)
	}
	if _, ok := registered.Data["id"].(string); !ok {
		t.Fatalf("register data = %#v, want account object", registered.Data)
	}

	login := request(t, http.MethodPost, `{"cccdNumber":"001234567890","password":"password123"}`, handler.Login)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200", login.Code)
	}
	loggedIn := decodeSuccess[map[string]any](t, login)
	if loggedIn.Status != http.StatusOK {
		t.Fatalf("login body status = %d, want 200", loggedIn.Status)
	}
	if token, ok := loggedIn.Data["token"].(string); !ok || token == "" {
		t.Fatalf("login data = %#v, want token", loggedIn.Data)
	}
	token := loggedIn.Data["token"].(string)
	accountID := registered.Data["id"].(string)

	mux := http.NewServeMux()
	handleraccount.RegisterRoutes(mux, handler, tokenService)

	accounts := routeRequest(t, mux, http.MethodGet, "/api/accounts", "", token)
	if accounts.Code != http.StatusOK {
		t.Fatalf("get all status = %d, want 200", accounts.Code)
	}
	list := decodeSuccess[[]map[string]any](t, accounts)
	if len(list.Data) != 1 {
		t.Fatalf("account count = %d, want 1", len(list.Data))
	}

	me := routeRequest(t, mux, http.MethodGet, "/api/auth/me", "", token)
	if me.Code != http.StatusOK {
		t.Fatalf("me status = %d, want 200", me.Code)
	}
	private := decodeSuccess[map[string]any](t, me)
	if private.Data["id"] != accountID {
		t.Fatalf("me data = %#v, want account %q", private.Data, accountID)
	}

	verify := routeRequest(t, mux, http.MethodPost, "/api/accounts/"+accountID+"/verify", `{
		"idCardFrontUrl":"kyc/front.jpg",
		"idCardBackUrl":"kyc/back.jpg",
		"liveVideoUrl":"kyc/live.webm"
	}`, token)
	if verify.Code != http.StatusOK {
		t.Fatalf("verify status = %d, want 200", verify.Code)
	}
	verified := decodeSuccess[map[string]any](t, verify)
	if verified.Data["isVerify"] != true {
		t.Fatalf("verify data = %#v, want verified account", verified.Data)
	}

	invalidLogin := request(t, http.MethodPost, `{"cccdNumber":"001234567890","password":"wrongpass"}`, handler.Login)
	if invalidLogin.Code != http.StatusUnauthorized {
		t.Fatalf("invalid login status = %d, want 401", invalidLogin.Code)
	}
	var errorResponse httpx.ErrorResponse
	decodeJSON(t, invalidLogin, &errorResponse)
	if errorResponse.Status != http.StatusUnauthorized || len(errorResponse.Errors) != 1 {
		t.Fatalf("invalid login error = %#v, want unauthorized envelope", errorResponse)
	}
	if errorResponse.Errors[0].Code != httpx.ErrorCodeInvalidCredentials {
		t.Fatalf("invalid login code = %q, want %q", errorResponse.Errors[0].Code, httpx.ErrorCodeInvalidCredentials)
	}
}

func decodeSuccess[T any](t *testing.T, recorder *httptest.ResponseRecorder) httpx.SuccessResponse[T] {
	t.Helper()
	var response httpx.SuccessResponse[T]
	decodeJSON(t, recorder, &response)
	return response
}

func decodeJSON(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

var _ composition.AccountRepository = (*memoryAccountRepository)(nil)
