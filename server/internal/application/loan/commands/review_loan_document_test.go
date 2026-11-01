package commands

import (
	"context"
	"errors"
	"testing"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
)

type documentReviewRepo struct {
	app      *loandomain.Application
	appended []string
}

func (r *documentReviewRepo) FindByIDWithRevision(context.Context, string) (*loandomain.Application, uint64, error) {
	return r.app, 3, nil
}

func (r *documentReviewRepo) Append(_ context.Context, _ string, revision uint64, eventType string, _ any) error {
	if revision != 3 {
		return errors.New("revision conflict")
	}
	r.appended = append(r.appended, eventType)
	return nil
}

type documentReviewAuth struct{ err error }

func (a documentReviewAuth) AuthorizeLoanReview(context.Context, string) error { return a.err }

func reviewFixture(status loandomain.ApplicationStatus) *documentReviewRepo {
	return &documentReviewRepo{app: &loandomain.Application{
		Id: "app", Status: status,
		Documents: []loandomain.ApplicationDocument{{DocumentId: "doc", VerificationStatus: "pending"}},
	}}
}

func TestReviewDocumentApprovesAndRejects(t *testing.T) {
	repo := reviewFixture(loandomain.ApplicationStatusUnderReview)
	handler := &ReviewLoanDocumentCommandHandler{Repo: repo, Authorizer: documentReviewAuth{}}

	result, err := handler.Handle(context.Background(), ReviewLoanDocumentCommand{DocumentID: "doc", Decision: "verified", ActorID: "admin"})
	if err != nil || result.Documents[0].VerificationStatus != "verified" || len(repo.appended) != 1 {
		t.Fatalf("result = %#v, err = %v, events = %v", result, err, repo.appended)
	}
	if repo.app.Documents[0].VerificationStatus != "pending" {
		t.Fatal("source application changed before the event was replayed")
	}

	result, err = handler.Handle(context.Background(), ReviewLoanDocumentCommand{DocumentID: "doc", Decision: "rejected", Reason: " Ảnh mờ ", ActorID: "admin"})
	if err != nil || result.Documents[0].RejectionReason != "Ảnh mờ" {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
}

func TestReviewDocumentRejectsInvalidRequests(t *testing.T) {
	for name, tc := range map[string]struct {
		status loandomain.ApplicationStatus
		auth   error
		cmd    ReviewLoanDocumentCommand
	}{
		"not under review": {loandomain.ApplicationStatusSubmitted, nil, ReviewLoanDocumentCommand{DocumentID: "doc", Decision: "verified", ActorID: "a"}},
		"not admin":        {loandomain.ApplicationStatusUnderReview, errors.New("denied"), ReviewLoanDocumentCommand{DocumentID: "doc", Decision: "verified", ActorID: "a"}},
		"no reason":        {loandomain.ApplicationStatusUnderReview, nil, ReviewLoanDocumentCommand{DocumentID: "doc", Decision: "rejected", ActorID: "a"}},
		"bad decision":     {loandomain.ApplicationStatusUnderReview, nil, ReviewLoanDocumentCommand{DocumentID: "doc", Decision: "pending", ActorID: "a"}},
		"unknown document": {loandomain.ApplicationStatusUnderReview, nil, ReviewLoanDocumentCommand{DocumentID: "x", Decision: "verified", ActorID: "a"}},
	} {
		repo := reviewFixture(tc.status)
		handler := &ReviewLoanDocumentCommandHandler{Repo: repo, Authorizer: documentReviewAuth{tc.auth}}
		if _, err := handler.Handle(context.Background(), tc.cmd); err == nil || len(repo.appended) != 0 {
			t.Errorf("%s: err = %v, events = %v", name, err, repo.appended)
		}
	}
}
