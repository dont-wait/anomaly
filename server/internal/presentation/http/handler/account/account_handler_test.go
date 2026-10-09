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
	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
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
	user, ok := registered.Data["user"].(map[string]any)
	if !ok || user["isVerify"] != false {
		t.Fatalf("register data = %#v, want unverified user", registered.Data)
	}
	kycToken, ok := registered.Data["kycToken"].(string)
	if !ok || kycToken == "" {
		t.Fatalf("register data = %#v, want KYC token", registered.Data)
	}
	if _, ok := registered.Data["kycExpiresAt"].(string); !ok {
		t.Fatalf("register data = %#v, want KYC expiry", registered.Data)
	}

	login := request(t, http.MethodPost, `{"cccdNumber":"001234567890","password":"password123"}`, handler.Login)
	if login.Code != http.StatusForbidden {
		t.Fatalf("unverified login status = %d, want 403", login.Code)
	}
	var kycRequired httpx.ErrorResponse
	decodeJSON(t, login, &kycRequired)
	if kycRequired.Errors[0].Code != httpx.ErrorCodeKYCRequired {
		t.Fatalf("login code = %q, want KYC_REQUIRED", kycRequired.Errors[0].Code)
	}
	session := request(t, http.MethodPost, `{"cccdNumber":"001234567890","password":"password123"}`, handler.StartKYCSession)
	if session.Code != http.StatusOK {
		t.Fatalf("KYC session status = %d, want 200", session.Code)
	}
	resumed := decodeSuccess[map[string]any](t, session)
	if resumed.Data["kycToken"] == "" {
		t.Fatalf("KYC session data = %#v, want token", resumed.Data)
	}
	alice, _ := repo.FindByCCCDNumber(context.Background(), "001234567890")
	alice.Customer.KYCStatus = accountdomain.KYCStatusVerified
	verifiedSession := request(t, http.MethodPost, `{"cccdNumber":"001234567890","password":"password123"}`, handler.StartKYCSession)
	if verifiedSession.Code != http.StatusForbidden {
		t.Fatalf("verified KYC session status = %d, want 403", verifiedSession.Code)
	}
	login = request(t, http.MethodPost, `{"cccdNumber":"001234567890","password":"password123"}`, handler.Login)
	loggedIn := decodeSuccess[map[string]any](t, login)
	token := loggedIn.Data["token"].(string)
	accountID := user["id"].(string)

	adminRegister := request(t, http.MethodPost, `{
		"username":"admin",
		"cccdNumber":"009876543210",
		"cccdIssuedDate":"2020-01-01T00:00:00Z",
		"dob":"1990-01-01T00:00:00Z",
		"email":"admin@example.com",
		"password":"adminpass123"
	}`, handler.Register)
	if adminRegister.Code != http.StatusCreated {
		t.Fatalf("admin register status = %d, want 201", adminRegister.Code)
	}
	adminAccount, err := repo.FindByCCCDNumber(context.Background(), "009876543210")
	if err != nil {
		t.Fatalf("find admin account: %v", err)
	}
	adminAccount.Role = accountdomain.AccountRoleAdmin
	adminAccount.Customer.KYCStatus = accountdomain.KYCStatusVerified
	if err := repo.Save(context.Background(), adminAccount); err != nil {
		t.Fatalf("save admin role: %v", err)
	}
	adminLogin := request(t, http.MethodPost, `{"cccdNumber":"009876543210","password":"adminpass123"}`, handler.Login)
	if adminLogin.Code != http.StatusOK {
		t.Fatalf("admin login status = %d, want 200", adminLogin.Code)
	}
	adminAuth := decodeSuccess[map[string]any](t, adminLogin)
	if adminAuth.Data["user"].(map[string]any)["role"] != string(accountdomain.AccountRoleAdmin) {
		t.Fatalf("admin login user = %#v, want admin role", adminAuth.Data["user"])
	}
	adminToken := adminAuth.Data["token"].(string)

	mux := http.NewServeMux()
	router := openapi.NewRegistry(mux, false)
	handleraccount.RegisterRoutes(router, handler, tokenService)

	accountsAsUser := routeRequest(t, mux, http.MethodGet, "/api/accounts", "", token)
	if accountsAsUser.Code != http.StatusForbidden {
		t.Fatalf("get all as user status = %d, want 403", accountsAsUser.Code)
	}
	accounts := routeRequest(t, mux, http.MethodGet, "/api/accounts", "", adminToken)
	if accounts.Code != http.StatusOK {
		t.Fatalf("get all status = %d, want 200", accounts.Code)
	}
	list := decodeSuccess[[]map[string]any](t, accounts)
	if len(list.Data) != 2 {
		t.Fatalf("account count = %d, want 2", len(list.Data))
	}
	selfByID := routeRequest(t, mux, http.MethodGet, "/api/accounts/"+accountID, "", token)
	if selfByID.Code != http.StatusOK {
		t.Fatalf("get self by ID status = %d, want 200", selfByID.Code)
	}
	selfByEmail := routeRequest(t, mux, http.MethodGet, "/api/accounts/by-email/alice@example.com", "", token)
	if selfByEmail.Code != http.StatusOK {
		t.Fatalf("get self by email status = %d, want 200", selfByEmail.Code)
	}
	otherAsUser := routeRequest(t, mux, http.MethodGet, "/api/accounts/"+adminAccount.Id, "", token)
	if otherAsUser.Code != http.StatusNotFound {
		t.Fatalf("get other account as user status = %d, want 404", otherAsUser.Code)
	}
	otherByEmailAsUser := routeRequest(t, mux, http.MethodGet, "/api/accounts/by-email/admin@example.com", "", token)
	if otherByEmailAsUser.Code != http.StatusNotFound {
		t.Fatalf("get other account by email as user status = %d, want 404", otherByEmailAsUser.Code)
	}
	otherAsAdmin := routeRequest(t, mux, http.MethodGet, "/api/accounts/"+accountID, "", adminToken)
	if otherAsAdmin.Code != http.StatusOK {
		t.Fatalf("get user account as admin status = %d, want 200", otherAsAdmin.Code)
	}

	me := routeRequest(t, mux, http.MethodGet, "/api/auth/me", "", token)
	if me.Code != http.StatusOK {
		t.Fatalf("me status = %d, want 200", me.Code)
	}
	private := decodeSuccess[map[string]any](t, me)
	if private.Data["id"] != accountID {
		t.Fatalf("me data = %#v, want account %q", private.Data, accountID)
	}
	meWithKYC := routeRequest(t, mux, http.MethodGet, "/api/auth/me", "", kycToken)
	if meWithKYC.Code != http.StatusUnauthorized {
		t.Fatalf("me with KYC token status = %d, want 401", meWithKYC.Code)
	}

	oldVerify := routeRequest(t, mux, http.MethodPost, "/api/kyc/verify", `{}`, kycToken)
	if oldVerify.Code != http.StatusNotFound {
		t.Fatalf("old verify status = %d, want 404", oldVerify.Code)
	}
	idempotentComplete := routeRequest(t, mux, http.MethodPost, "/api/kyc/complete", "", kycToken)
	if idempotentComplete.Code != http.StatusOK {
		t.Fatalf("idempotent complete status = %d, want 200", idempotentComplete.Code)
	}
	completeResult := decodeSuccess[map[string]any](t, idempotentComplete)
	if completeResult.Data["decision"] != "VERIFIED" {
		t.Fatalf("idempotent complete = %#v", completeResult.Data)
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

func TestVerifiedRegistrationReplayDoesNotMintKYCToken(t *testing.T) {
	repo := newMemoryAccountRepository()
	tokens := auth.NewTokenService("test-secret", time.Hour)
	handler := composition.NewAccountHandler(repo, tokens, zerolog.Nop())
	body := `{
		"idempotencyKey":"f2c758e3-eaed-4c70-b322-d630fe934c2f",
		"username":"replay-user","cccdNumber":"111222333444",
		"cccdIssuedDate":"2020-01-01T00:00:00Z","dob":"1990-01-01T00:00:00Z",
		"email":"replay@example.com","password":"password123"
	}`
	first := request(t, http.MethodPost, body, handler.Register)
	if first.Code != http.StatusCreated {
		t.Fatalf("first registration = %d", first.Code)
	}
	account, err := repo.FindByCCCDNumber(context.Background(), "111222333444")
	if err != nil {
		t.Fatal(err)
	}
	account.Customer.KYCStatus = accountdomain.KYCStatusVerified
	replay := request(t, http.MethodPost, body, handler.Register)
	if replay.Code != http.StatusCreated {
		t.Fatalf("registration replay = %d, body = %s", replay.Code, replay.Body.String())
	}
	response := decodeSuccess[map[string]any](t, replay)
	if _, exists := response.Data["kycToken"]; exists {
		t.Fatalf("verified replay minted KYC token: %#v", response.Data)
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
