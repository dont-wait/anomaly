package commands

import (
	"context"
	"errors"
	"testing"
	"time"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
)

type rejectReviewAuth struct{ err error }

func (a rejectReviewAuth) AuthorizeLoanManualDecision(context.Context, string) error { return a.err }

func TestRejectLoanReview(t *testing.T) {
	repo := reviewFixture(loandomain.ApplicationStatusUnderReview)
	handler := &RejectLoanReviewCommandHandler{Repo: repo, Authorizer: rejectReviewAuth{}}

	result, err := handler.Handle(context.Background(), "app", "admin", "Chứng từ giả")
	if err != nil || result.Status != loandomain.ApplicationStatusRejected {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	if len(repo.appended) != 1 || repo.appended[0] != loanReviewRejectedEvent {
		t.Fatalf("events = %v", repo.appended)
	}
	if repo.app.Status != loandomain.ApplicationStatusUnderReview {
		t.Fatal("source application changed before the event was replayed")
	}
}

func TestRejectLoanReviewGuards(t *testing.T) {
	denied := errors.New("unauthorized")
	repo := reviewFixture(loandomain.ApplicationStatusUnderReview)
	handler := &RejectLoanReviewCommandHandler{Repo: repo, Authorizer: rejectReviewAuth{err: denied}}
	if _, err := handler.Handle(context.Background(), "app", "user", "x"); !errors.Is(err, denied) {
		t.Fatalf("err = %v", err)
	}

	assessed := reviewFixture(loandomain.ApplicationStatusUnderReview)
	now := time.Now()
	assessed.app.AssessedAt = &now
	handler = &RejectLoanReviewCommandHandler{Repo: assessed, Authorizer: rejectReviewAuth{}}
	if _, err := handler.Handle(context.Background(), "app", "admin", "x"); !errors.Is(err, loandomain.ErrInvalidApplicationStatus) {
		t.Fatalf("err = %v", err)
	}
	if len(assessed.appended) != 0 {
		t.Fatal("nothing should be appended")
	}
}
