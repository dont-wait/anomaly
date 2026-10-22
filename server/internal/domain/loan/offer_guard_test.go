package loan

import (
	"errors"
	"testing"
	"time"
)

func TestNewDisbursementRequiresConfirmedOffer(t *testing.T) {
	a := approvedOfferTestApplication(t)
	err := a.MarkDisbursed(time.Now().UTC(), 1600)
	if !errors.Is(err, ErrLoanContractNotConfirmed) {
		t.Fatalf("expected contract guard, got %v", err)
	}
	if a.DisbursedAt != nil || a.Status != ApplicationStatusApproved {
		t.Fatal("rejected disbursement mutated loan")
	}
}
