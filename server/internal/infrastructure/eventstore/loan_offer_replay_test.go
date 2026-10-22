package eventstore

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
)

func TestLoanOfferReplayAndLegacyCredit(t *testing.T) {
	a := &loandomain.Application{}
	created := time.Now().UTC().Add(-time.Hour)
	draft, err := loandomain.NewDraftApplication("loan-1", "LA-1", "owner-1", "salary", "account-1", 10_000_000, 12)
	if err != nil {
		t.Fatal(err)
	}
	draft.CreatedAt = created
	draft.UpdatedAt = created
	mustReplayLoanEvent(t, a, EventLoanApplicationCreated, loanApplicationCreatedPayload{Application: draft})
	// Prior approved events remain replayable without loan-offer events.
	a.Status = loandomain.ApplicationStatusApproved
	approvedAt := created.Add(time.Minute)
	a.ApprovedAt = &approvedAt
	issuedAt := approvedAt.Add(time.Second)
	mustReplayLoanEvent(t, a, EventLoanOfferIssued, loanOfferIssuedPayload{IssuedAt: issuedAt, AnnualRateBps: 1600, UpdatedAt: issuedAt})
	at := issuedAt.Add(time.Second)
	mustReplayLoanEvent(t, a, EventLoanOfferAccepted, loanOfferTransitionPayload{At: at, UpdatedAt: at})
	at = at.Add(time.Second)
	mustReplayLoanEvent(t, a, EventLoanContractConfirmed, loanOfferTransitionPayload{At: at, UpdatedAt: at, Method: "email_otp"})
	if a.Offer == nil || a.Offer.Status != loandomain.LoanOfferConfirmed {
		t.Fatal("replay lost confirmation")
	}
	before, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyLoanEvent(a, EventLoanOfferAccepted, loanReplayJSON(t, loanOfferTransitionPayload{At: at.Add(time.Second), UpdatedAt: at.Add(time.Second)})); err == nil {
		t.Fatal("duplicate acceptance replay passed")
	}
	after, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("rejected replay mutated aggregate: before=%s after=%s", before, after)
	}
	if !errors.Is(applyLoanEvent(a, "FutureLoanEvent", []byte(`{}`)), ErrLoanUnsupportedEvent) {
		t.Fatal("unknown version must be typed unsupported-event error")
	}
}
