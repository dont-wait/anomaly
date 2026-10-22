package commands

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/dont-wait/anomaly/internal/domain/loan"
)

type offerRetryRepo struct {
	LoanDocumentRepository
	app      *domain.Application
	revision uint64
	writes   int
}

func (r *offerRetryRepo) FindByIDWithRevision(_ context.Context, id string) (*domain.Application, uint64, error) {
	if r.app == nil || r.app.Id != id {
		return nil, 0, nil
	}
	return r.app, r.revision, nil
}
func (r *offerRetryRepo) Append(_ context.Context, id string, rev uint64, kind string, payload any) error {
	if id != r.app.Id || rev != r.revision {
		return errors.New("revision conflict")
	}

	eventData, ok := payload.(struct {
		At        time.Time `json:"at"`
		UpdatedAt time.Time `json:"updatedAt"`
		Method    string    `json:"method,omitempty"`
	})
	if !ok {
		return errors.New("invalid loan offer event payload")
	}
	at := eventData.At

	switch kind {
	case "LoanOfferAccepted":
		if err := r.app.AcceptOffer(at); err != nil {
			return err
		}
	case "LoanOfferRejected":
		if err := r.app.RejectOffer(at); err != nil {
			return err
		}
	case "LoanContractConfirmed":
		if err := r.app.ConfirmContract(at, "email_otp"); err != nil {
			return err
		}
	default:
		return errors.New("unexpected event")
	}
	r.writes++
	r.revision++
	return nil
}
func newRetryOffer(t *testing.T) *domain.Application {
	t.Helper()
	a, err := domain.NewDraftApplication("loan-retry", "LA-retry", "owner", "PERSONAL_UNSECURED_SALARY", "account", 10_000_000, 12)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Add(-time.Minute)
	a.Status = domain.ApplicationStatusApproved
	a.ApprovedAt = &at
	if err := a.IssueOffer(at.Add(time.Second), 1600); err != nil {
		t.Fatal(err)
	}
	return a
}
func TestLoanOfferAcceptAndConfirmAreIdempotent(t *testing.T) {
	repo := &offerRetryRepo{app: newRetryOffer(t), revision: 10}
	handler := &LoanOfferCommandHandler{Repo: repo}
	ctx := context.Background()
	if _, err := handler.Accept(ctx, repo.app.Id, "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := handler.Accept(ctx, repo.app.Id, "owner"); err != nil {
		t.Fatal(err)
	}
	if repo.writes != 1 {
		t.Fatalf("accept retry wrote %d events", repo.writes)
	}
	if _, err := handler.Confirm(ctx, repo.app.Id, "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := handler.Confirm(ctx, repo.app.Id, "owner"); err != nil {
		t.Fatal(err)
	}
	if repo.writes != 2 {
		t.Fatalf("confirm retry wrote %d events", repo.writes)
	}
	if _, err := handler.Accept(ctx, repo.app.Id, "not-owner"); err == nil {
		t.Fatal("unauthorized retry succeeded")
	}
	if _, err := handler.Reject(ctx, repo.app.Id, "owner"); err == nil {
		t.Fatal("rejected confirmed offer")
	}
}
func TestLoanOfferRejectRetryIsIdempotent(t *testing.T) {
	repo := &offerRetryRepo{app: newRetryOffer(t), revision: 10}
	handler := &LoanOfferCommandHandler{Repo: repo}
	ctx := context.Background()
	if _, err := handler.Reject(ctx, repo.app.Id, "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := handler.Reject(ctx, repo.app.Id, "owner"); err != nil {
		t.Fatal(err)
	}
	if repo.writes != 1 {
		t.Fatalf("reject retry wrote %d events", repo.writes)
	}
	if _, err := handler.Accept(ctx, repo.app.Id, "owner"); err == nil {
		t.Fatal("accepted rejected offer")
	}
}
