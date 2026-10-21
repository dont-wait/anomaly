package loan

import (
	"errors"
	"testing"
	"time"
)

func approvedOfferTestApplication(t *testing.T) *Application {
	t.Helper()
	a, err := NewDraftApplication("loan-1", "LA-1", "customer-1", "PERSONAL_UNSECURED_SALARY", "account-1", 10_000_000, 12)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-time.Minute)
	a.Status = ApplicationStatusApproved
	a.ApprovedAt = &now
	return a
}

func TestLoanOfferLifecycle(t *testing.T) {
	a := approvedOfferTestApplication(t)
	at := time.Now().UTC()
	if err := a.IssueOffer(at, 1600); err != nil {
		t.Fatal(err)
	}
	if a.Offer.Amount != a.RequestedAmount || a.Offer.AnnualRateBps != 1600 {
		t.Fatal("offer terms mismatch")
	}
	if err := a.IssueOffer(at, 1700); err == nil {
		t.Fatal("offer terms changed")
	}
	if err := a.ConfirmContract(at.Add(time.Second), "email_otp"); !errors.Is(err, ErrLoanOfferNotAccepted) {
		t.Fatalf("want acceptance: %v", err)
	}
	if err := a.AcceptOffer(at.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := a.ConfirmContract(at.Add(2*time.Second), "accept_terms"); !errors.Is(err, ErrLoanContractNotConfirmed) {
		t.Fatal("confirmation must need OTP")
	}
	if err := a.ConfirmContract(at.Add(2*time.Second), "email_otp"); err != nil {
		t.Fatal(err)
	}
	if a.Offer.Status != LoanOfferConfirmed || a.Offer.ConfirmedAt == nil {
		t.Fatal("contract not confirmed")
	}
	if err := a.AcceptOffer(at.Add(3 * time.Second)); err == nil {
		t.Fatal("duplicate acceptance allowed")
	}
}

func TestLoanOfferRejectBlocksAcceptance(t *testing.T) {
	a := approvedOfferTestApplication(t)
	at := time.Now().UTC()
	if err := a.IssueOffer(at, 1600); err != nil {
		t.Fatal(err)
	}
	if err := a.RejectOffer(at.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := a.AcceptOffer(at.Add(2 * time.Second)); err == nil {
		t.Fatal("rejected offer accepted")
	}
}
