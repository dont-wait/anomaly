package commands

import (
	"context"
	"errors"
	"testing"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
)

func TestDeleteLoanApplication(t *testing.T) {
	repo := &attachRepo{app: &loandomain.Application{Id: "app", CustomerId: "me", Status: loandomain.ApplicationStatusDraft}}
	handler := &DeleteLoanApplicationCommandHandler{Repo: repo}

	if err := handler.Handle(context.Background(), "app", "other"); err == nil || len(repo.revisions) != 0 {
		t.Fatalf("other customer: err = %v, revisions = %v", err, repo.revisions)
	}
	if err := handler.Handle(context.Background(), "app", "me"); err != nil || len(repo.revisions) != 1 {
		t.Fatalf("owner: err = %v, revisions = %v", err, repo.revisions)
	}

	repo.app.Status = loandomain.ApplicationStatusSubmitted
	if err := handler.Handle(context.Background(), "app", "me"); !errors.Is(err, loandomain.ErrInvalidApplicationStatus) {
		t.Fatalf("submitted: err = %v", err)
	}
}
