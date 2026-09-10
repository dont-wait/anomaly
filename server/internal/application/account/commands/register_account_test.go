package commands

import (
	"context"
	"errors"
	"testing"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type registrationRepository struct {
	verifyAccountRepository
	creates int
}

func (r *registrationRepository) FindByID(_ context.Context, id string) (*accountdomain.UserAccount, error) {
	if r.account != nil && r.account.Id == id {
		return r.account, nil
	}
	return nil, nil
}

func (r *registrationRepository) FindByEmail(context.Context, string) (*accountdomain.UserAccount, error) {
	return r.account, nil
}

func (r *registrationRepository) Create(_ context.Context, acc *accountdomain.UserAccount) error {
	r.creates++
	r.account = acc
	return nil
}

func TestRegistrationReplayAfterLostResponse(t *testing.T) {
	repo := &registrationRepository{}
	cmd := RegisterAccountCommand{
		IdempotencyKey: "b5b552a4-f8b7-43c3-855d-c2c7903a5af2",
		Username:       "Example", Email: "example@example.com", CCCDNumber: "012345678901",
		DOB:            time.Date(1995, 1, 1, 0, 0, 0, 0, time.UTC),
		CCCDIssuedDate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), Password: "Strong123!",
	}
	original, err := NewRegisterAccountCommandHandler(repo, repo).Handle(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	// A new handler has no in-memory request cache. Only the persisted account remains.
	repo.account.Balance.Current = 100
	replay, err := NewRegisterAccountCommandHandler(repo, repo).Handle(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Id != original.Id || repo.creates != 1 || replay.Balance.Current != 0 || repo.account.Balance.Current != 100 {
		t.Fatal("retry did not preserve the original registration result")
	}
	for _, mutate := range []func(*RegisterAccountCommand){
		func(c *RegisterAccountCommand) { c.Password = "Different123!" },
		func(c *RegisterAccountCommand) { c.Email = "different@example.com" },
		func(c *RegisterAccountCommand) { c.Username = "Different" },
		func(c *RegisterAccountCommand) { c.CCCDNumber = "012345678902" },
		func(c *RegisterAccountCommand) { c.DOB = c.DOB.AddDate(1, 0, 0) },
		func(c *RegisterAccountCommand) { c.CCCDIssuedDate = c.CCCDIssuedDate.AddDate(1, 0, 0) },
	} {
		changed := cmd
		mutate(&changed)
		_, err := NewRegisterAccountCommandHandler(repo, repo).Handle(context.Background(), changed)
		if !errors.Is(err, accountdomain.ErrIdempotencyConflict) {
			t.Fatalf("changed payload error = %v", err)
		}
	}
	cmd.IdempotencyKey = "bbaf71ec-9d49-4c83-9b41-8072510f9b78"
	if _, err := NewRegisterAccountCommandHandler(repo, repo).Handle(context.Background(), cmd); !errors.Is(err, accountdomain.ErrUserAlreadyExists) {
		t.Fatalf("different key should conflict, got %v", err)
	}
	if repo.creates != 1 {
		t.Fatalf("Create calls = %d", repo.creates)
	}
}
