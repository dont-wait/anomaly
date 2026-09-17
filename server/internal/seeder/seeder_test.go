package seeder

import (
	"context"
	"errors"
	"testing"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"golang.org/x/crypto/bcrypt"
)

type memoryRepository struct {
	account        *accountdomain.UserAccount
	creates, saves int
	saveErr        error
}

func (r *memoryRepository) FindByID(context.Context, string) (*accountdomain.UserAccount, error) {
	return r.account, nil
}

func (r *memoryRepository) FindByCCCDNumber(context.Context, string) (*accountdomain.UserAccount, error) {
	return r.account, nil
}

func (r *memoryRepository) FindByEmail(context.Context, string) (*accountdomain.UserAccount, error) {
	return r.account, nil
}

func (r *memoryRepository) FindByUsername(context.Context, string) (*accountdomain.UserAccount, error) {
	return r.account, nil
}

func (r *memoryRepository) Create(_ context.Context, a *accountdomain.UserAccount) error {
	r.account = a
	r.creates++
	return nil
}

func (r *memoryRepository) Save(_ context.Context, a *accountdomain.UserAccount) error {
	r.saves++
	if r.saveErr != nil {
		return r.saveErr
	}
	r.account = a
	return nil
}

func TestRunCreatesHashedDemoAndCanRepeat(t *testing.T) {
	repo := &memoryRepository{}
	ctx := context.Background()
	if err := Run(ctx, Dependencies{Accounts: repo}); err != nil {
		t.Fatal(err)
	}
	seed := demoAccounts()[0]
	if repo.account.Balance.Current != seed.Balance {
		t.Fatal("wrong demo balance")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(repo.account.PasswordHash), []byte(seed.Password)); err != nil {
		t.Fatal(err)
	}
	if err := Run(ctx, Dependencies{Accounts: repo}); err != nil {
		t.Fatal(err)
	}
	if repo.creates != 1 || repo.saves != 1 {
		t.Fatalf("creates=%d saves=%d", repo.creates, repo.saves)
	}
	repo.account.Balance.Current = 0
	version := repo.account.Version
	if err := Run(ctx, Dependencies{Accounts: repo}); err != nil {
		t.Fatal(err)
	}
	if repo.account.Balance.Current != seed.Balance || repo.account.Version != version+1 {
		t.Fatal("balance was not reconciled")
	}
}

func TestRunRejectsExistingPasswordCollision(t *testing.T) {
	repo := &memoryRepository{}
	if err := Run(context.Background(), Dependencies{Accounts: repo}); err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("different-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	repo.account.PasswordHash = string(hash)
	repo.account.Balance.Current = 1
	if err := Run(context.Background(), Dependencies{Accounts: repo}); err == nil {
		t.Fatal("expected collision error")
	}
	if repo.saves != 1 || repo.account.Balance.Current != 1 {
		t.Fatal("collision modified existing account")
	}
}

func TestRunPropagatesSaveFailure(t *testing.T) {
	failure := errors.New("save failed")
	repo := &memoryRepository{saveErr: failure}
	if err := Run(context.Background(), Dependencies{Accounts: repo}); !errors.Is(err, failure) {
		t.Fatalf("expected save error, got %v", err)
	}
}
