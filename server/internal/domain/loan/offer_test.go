package loan

import (
	"bytes"
	"encoding/json"
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
	unchangedOffer := snapshotLoanOffer(t, a)
	if err := a.IssueOffer(at, 1700); err == nil {
		t.Fatal("offer terms changed")
	}
	assertLoanOfferUnchanged(t, a, unchangedOffer)
	unchangedOffer = snapshotLoanOffer(t, a)
	if err := a.ConfirmContract(at.Add(time.Second), "email_otp"); !errors.Is(err, ErrLoanOfferNotAccepted) {
		t.Fatalf("want acceptance: %v", err)
	}
	assertLoanOfferUnchanged(t, a, unchangedOffer)
	if err := a.AcceptOffer(at.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	unchangedOffer = snapshotLoanOffer(t, a)
	if err := a.ConfirmContract(at.Add(2*time.Second), "accept_terms"); !errors.Is(err, ErrLoanContractNotConfirmed) {
		t.Fatal("confirmation must need OTP")
	}
	assertLoanOfferUnchanged(t, a, unchangedOffer)
	if err := a.ConfirmContract(at.Add(2*time.Second), "email_otp"); err != nil {
		t.Fatal(err)
	}
	if a.Offer.Status != LoanOfferConfirmed || a.Offer.ConfirmedAt == nil {
		t.Fatal("contract not confirmed")
	}
	unchangedOffer = snapshotLoanOffer(t, a)
	if err := a.AcceptOffer(at.Add(3 * time.Second)); err == nil {
		t.Fatal("duplicate acceptance allowed")
	}
	assertLoanOfferUnchanged(t, a, unchangedOffer)
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

func snapshotLoanOffer(t *testing.T, a *Application) []byte {
	t.Helper()
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func assertLoanOfferUnchanged(t *testing.T, a *Application, before []byte) {
	t.Helper()
	after := snapshotLoanOffer(t, a)
	if !bytes.Equal(before, after) {
		t.Fatalf("rejected operation mutated aggregate: before=%s after=%s", before, after)
	}
}
