package admin

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/audit"
	savingsdomain "github.com/dont-wait/anomaly/internal/domain/savings"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/google/uuid"
	esdb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
)

func TestParseEventExplorerRangeSupportsDateBounds(t *testing.T) {
	from, to, err := parseEventExplorerRange("2026-10-01", "2026-10-30")
	if err != nil {
		t.Fatalf("parse range: %v", err)
	}
	if !from.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("from = %s", from)
	}
	if !to.Equal(time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("to = %s", to)
	}
}

func TestEventExplorerCursorRoundTripKeepsFilters(t *testing.T) {
	want := eventExplorerCursor{Version: eventExplorerCursorVersion, Commit: 10, Prepare: 12, QueryKey: "query"}
	encoded, err := encodeEventExplorerCursor(want)
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	got, err := decodeEventExplorerCursor(encoded)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	if got != want {
		t.Fatalf("cursor = %+v, want %+v", got, want)
	}
}

func TestBuildEventExplorerItemMasksAndClassifiesTransfer(t *testing.T) {
	eventID := uuid.New()
	transactionID := uuid.NewString()
	payload, err := json.Marshal(journal.Record{
		ID:   eventID.String(),
		Type: journal.Completed,
		At:   time.Date(2026, 10, 30, 7, 30, 0, 0, time.UTC),
		Event: &journal.Event{
			ID:            eventID.String(),
			TransactionID: transactionID,
			Type:          journal.Completed,
			OccurredAt:    time.Date(2026, 10, 30, 7, 30, 0, 0, time.UTC),
			Payload: journal.Payload{
				SourceAccountID:      "account-source",
				DestinationAccountID: "account-destination",
				Amount:               500000,
				Currency:             "VND",
				Channel:              "desktop",
				Status:               "success",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}

	item, ok := buildEventExplorerItem(&esdb.RecordedEvent{
		EventID:     eventID,
		EventType:   journal.Completed,
		StreamID:    journal.BankStream,
		EventNumber: 12,
		Data:        payload,
	})
	if !ok {
		t.Fatal("transfer event was not classified")
	}
	if item.Category != "transfer" || item.TransactionID != transactionID || len(item.AccountIDs) != 2 || item.AccountRoles["account-source"] != "Gửi" || item.AccountRoles["account-destination"] != "Nhận" {
		t.Fatalf("item = %+v", item)
	}
	if _, exposed := item.Details["riskAssessment"]; exposed {
		t.Fatal("risk assessment payload should not be exposed")
	}
}

func TestBuildEventExplorerItemInfersAuditAccountFromStream(t *testing.T) {
	event := audit.NewEvent(audit.LoginFailed, "operation-1", time.Date(2026, 10, 30, 7, 30, 0, 0, time.UTC))
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	item, ok := buildEventExplorerItem(&esdb.RecordedEvent{
		EventID:     uuid.MustParse(event.ID),
		EventType:   event.Type,
		StreamID:    auditAccountPrefix + "account-1",
		EventNumber: 3,
		Data:        payload,
	})
	if !ok || item.Category != "login" || len(item.AccountIDs) != 1 || item.AccountIDs[0] != "account-1" {
		t.Fatalf("item = %+v, ok = %v", item, ok)
	}
}

func TestBuildEventExplorerItemSupportsAccountStream(t *testing.T) {
	item, ok := buildEventExplorerItem(&esdb.RecordedEvent{
		EventID:     uuid.New(),
		EventType:   "BankAccountCreated",
		StreamID:    eventExplorerAccountPrefix + "account-1",
		EventNumber: 1,
		CreatedDate: time.Date(2026, 10, 30, 7, 30, 0, 0, time.UTC),
	})
	if !ok || item.Category != "account" || len(item.AccountIDs) != 1 || item.AccountIDs[0] != "account-1" {
		t.Fatalf("item = %+v, ok = %v", item, ok)
	}
}

func TestBuildEventExplorerItemExtractsSavingsTransactionID(t *testing.T) {
	payload, err := json.Marshal(journal.Record{
		ID:        "savings-event-1",
		Type:      journal.SavingsOpened,
		At:        time.Date(2026, 10, 30, 7, 30, 0, 0, time.UTC),
		AccountID: "account-1",
		Savings: &journal.SavingsMovement{
			TransactionID: "savings-tx-1",
			Contract: savingsdomain.Contract{
				ID:              "savings-1",
				SourceAccountID: "account-1",
				Principal:       1000000,
				Status:          savingsdomain.ContractActive,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}
	item, ok := buildEventExplorerItem(&esdb.RecordedEvent{
		EventID:     uuid.New(),
		EventType:   journal.SavingsOpened,
		StreamID:    journal.BankStream,
		EventNumber: 4,
		Data:        payload,
	})
	if !ok || item.TransactionID != "savings-tx-1" || item.Category != "other" {
		t.Fatalf("item = %+v, ok = %v", item, ok)
	}
}

func TestBuildEventExplorerItemExtractsLoanPaymentTransactionID(t *testing.T) {
	payload, err := json.Marshal(journal.Record{
		ID:   "loan-payment-event-1",
		Type: journal.LoanInstallmentPaid,
		At:   time.Date(2026, 10, 30, 7, 30, 0, 0, time.UTC),
		LoanPayment: &journal.LoanInstallmentPayment{
			TransactionID: "loan-payment-tx-1",
			LoanID:        "loan-1",
			AccountID:     "account-1",
			Amount:        250000,
		},
	})
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}
	item, ok := buildEventExplorerItem(&esdb.RecordedEvent{
		EventID:     uuid.New(),
		EventType:   journal.LoanInstallmentPaid,
		StreamID:    journal.BankStream,
		EventNumber: 5,
		Data:        payload,
	})
	if !ok || item.TransactionID != "loan-payment-tx-1" || item.LoanID != "loan-1" || item.Category != "loan" {
		t.Fatalf("item = %+v, ok = %v", item, ok)
	}
}

func TestEventExplorerQueryRejectsCursorForDifferentFilters(t *testing.T) {
	request := httptest.NewRequest("GET", "/api/admin/audit/events?category=login", nil)
	query, err := parseEventExplorerQuery(request)
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	if eventExplorerQueryKey(query) == eventExplorerQueryKey(eventExplorerQuery{Category: "transfer", Limit: eventExplorerDefaultLimit}) {
		t.Fatal("different filters should produce different cursor keys")
	}
}
