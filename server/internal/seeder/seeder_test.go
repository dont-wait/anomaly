package seeder

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"golang.org/x/crypto/bcrypt"
)

type memoryRepository struct {
	accounts       map[string]*accountdomain.UserAccount
	creates, saves int
	saveErr        error
}

func (r *memoryRepository) FindByID(_ context.Context, id string) (*accountdomain.UserAccount, error) {
	for _, account := range r.accounts {
		if account.Id == id {
			return cloneAccount(account), nil
		}
	}
	return nil, nil
}

func (r *memoryRepository) FindByCCCDNumber(_ context.Context, cccd string) (*accountdomain.UserAccount, error) {
	for _, account := range r.accounts {
		if account.Customer != nil && account.Customer.Identity.Number == cccd {
			return cloneAccount(account), nil
		}
	}
	return nil, nil
}

func (r *memoryRepository) FindByEmail(_ context.Context, email string) (*accountdomain.UserAccount, error) {
	for _, account := range r.accounts {
		if account.Email == email {
			return cloneAccount(account), nil
		}
	}
	return nil, nil
}

func (r *memoryRepository) FindByUsername(_ context.Context, username string) (*accountdomain.UserAccount, error) {
	for _, account := range r.accounts {
		if account.Username == username {
			return cloneAccount(account), nil
		}
	}
	return nil, nil
}

func (r *memoryRepository) Create(_ context.Context, a *accountdomain.UserAccount) error {
	if r.accounts == nil {
		r.accounts = map[string]*accountdomain.UserAccount{}
	}
	r.accounts[a.Customer.Identity.Number] = cloneAccount(a)
	r.creates++
	return nil
}

func (r *memoryRepository) Save(_ context.Context, a *accountdomain.UserAccount) error {
	r.saves++
	if r.saveErr != nil {
		return r.saveErr
	}
	if r.accounts == nil {
		r.accounts = map[string]*accountdomain.UserAccount{}
	}
	r.accounts[a.Customer.Identity.Number] = cloneAccount(a)
	return nil
}

func cloneAccount(account *accountdomain.UserAccount) *accountdomain.UserAccount {
	data, err := json.Marshal(account)
	if err != nil {
		panic(err)
	}
	var clone accountdomain.UserAccount
	if err := json.Unmarshal(data, &clone); err != nil {
		panic(err)
	}
	return &clone
}

func TestRunCreatesHashedDemoAndCanRepeat(t *testing.T) {
	t.Setenv("ANOMALY_DEMO_ADMIN_PASSWORD", "DemoAdmin@123")
	repo := &memoryRepository{}
	ctx := context.Background()
	if err := Run(ctx, Dependencies{Accounts: repo}); err != nil {
		t.Fatal(err)
	}
	seed := demoAccounts()[0]
	account := repo.accounts[seed.CCCD]
	if account.Balance.Current != seed.Balance {
		t.Fatal("wrong demo balance")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(seed.Password)); err != nil {
		t.Fatal(err)
	}
	adminSeed := demoAccounts()[1]
	admin := repo.accounts[adminSeed.CCCD]
	if admin.EffectiveRole() != accountdomain.AccountRoleAdmin {
		t.Fatalf("admin role = %q, want %q", admin.EffectiveRole(), accountdomain.AccountRoleAdmin)
	}
	if !admin.IsVerified() {
		t.Fatal("admin should be seeded as KYC verified")
	}
	if admin.VerifiedKYCSession() == nil {
		t.Fatal("admin should have a verified KYC session")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(adminSeed.Password)); err != nil {
		t.Fatalf("admin password hash: %v", err)
	}
	if err := Run(ctx, Dependencies{Accounts: repo}); err != nil {
		t.Fatal(err)
	}
	if repo.creates != 2 || repo.saves != 2 {
		t.Fatalf("creates=%d saves=%d", repo.creates, repo.saves)
	}
	account.Balance.Current = 0
	version := account.Version
	if err := Run(ctx, Dependencies{Accounts: repo}); err != nil {
		t.Fatal(err)
	}
	reconciled := repo.accounts[seed.CCCD]
	if reconciled.Balance.Current != seed.Balance || reconciled.Version != version+1 {
		t.Fatal("balance was not reconciled")
	}
}

func TestRunRejectsExistingPasswordCollision(t *testing.T) {
	t.Setenv("ANOMALY_DEMO_ADMIN_PASSWORD", "DemoAdmin@123")
	repo := &memoryRepository{}
	if err := Run(context.Background(), Dependencies{Accounts: repo}); err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("different-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	account := repo.accounts[demoAccounts()[0].CCCD]
	account.PasswordHash = string(hash)
	account.Balance.Current = 1
	err = Run(context.Background(), Dependencies{Accounts: repo})
	if err == nil {
		t.Fatal("expected collision error")
	}
	if !strings.Contains(err.Error(), "password") {
		t.Fatalf("collision error = %q, want password mismatch", err)
	}
	if repo.saves != 2 || account.Balance.Current != 1 {
		t.Fatal("collision modified existing account")
	}
}

func TestRunReconcilesAdminRole(t *testing.T) {
	t.Setenv("ANOMALY_DEMO_ADMIN_PASSWORD", "DemoAdmin@123")
	repo := &memoryRepository{}
	if err := Run(context.Background(), Dependencies{Accounts: repo}); err != nil {
		t.Fatal(err)
	}
	admin := repo.accounts[demoAccounts()[1].CCCD]
	admin.Role = accountdomain.AccountRoleUser
	admin.Customer.KYCStatus = accountdomain.KYCStatusNotStarted
	admin.Balance.Current = 42

	if err := Run(context.Background(), Dependencies{Accounts: repo}); err != nil {
		t.Fatal(err)
	}
	admin = repo.accounts[demoAccounts()[1].CCCD]
	if admin.Role != accountdomain.AccountRoleAdmin {
		t.Fatalf("admin role = %q, want %q", admin.Role, accountdomain.AccountRoleAdmin)
	}
	if !admin.IsVerified() {
		t.Fatal("existing admin should be reconciled as KYC verified")
	}
	if admin.Balance.Current != 42 {
		t.Fatalf("existing admin balance = %d, want 42", admin.Balance.Current)
	}
}

func TestRunDoesNotReconcileNonAdminKYCState(t *testing.T) {
	t.Setenv("ANOMALY_DEMO_ADMIN_PASSWORD", "")
	repo := &memoryRepository{}
	if err := Run(context.Background(), Dependencies{Accounts: repo}); err != nil {
		t.Fatal(err)
	}

	account := repo.accounts[demoAccounts()[0].CCCD]
	account.Customer.KYCStatus = accountdomain.KYCStatusVerified
	saves := repo.saves
	version := account.Version
	if err := Run(context.Background(), Dependencies{Accounts: repo}); err != nil {
		t.Fatal(err)
	}

	account = repo.accounts[demoAccounts()[0].CCCD]
	if repo.saves != saves {
		t.Fatalf("saves = %d, want %d", repo.saves, saves)
	}
	if account.Version != version || !account.IsVerified() {
		t.Fatalf("non-admin KYC state was reconciled: version=%d verified=%v", account.Version, account.IsVerified())
	}
}

func TestRunPropagatesSaveFailure(t *testing.T) {
	failure := errors.New("save failed")
	repo := &memoryRepository{saveErr: failure}
	if err := Run(context.Background(), Dependencies{Accounts: repo}); !errors.Is(err, failure) {
		t.Fatalf("expected save error, got %v", err)
	}
}
