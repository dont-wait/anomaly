package admin

import (
	"encoding/json"
	"testing"
	"time"

	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
)

func TestBankingTimelineItemLinksCompletedTransferToSourceAccount(t *testing.T) {
	accountID := "account-source"
	financialID := journal.FinancialAccountID(accountID)
	amount := int64(500000)
	fee := int64(1000)
	sourceBefore := int64(2000000)
	sourceAfter := int64(1499000)
	destinationBefore := int64(1000000)
	destinationAfter := int64(1500000)
	at := time.Date(2026, 10, 30, 7, 30, 0, 0, time.UTC)
	record := journal.Record{
		ID:   "event-id",
		Type: journal.Completed,
		At:   at,
		Event: &journal.Event{
			ID:            "event-id",
			TransactionID: "tx-1",
			Type:          journal.Completed,
			SchemaVersion: journal.SchemaVersion,
			Sequence:      2,
			OccurredAt:    at,
			Payload: journal.Payload{
				SourceAccountID:          financialID,
				DestinationAccountID:     journal.FinancialAccountID("account-destination"),
				Amount:                   amount,
				Fee:                      fee,
				Currency:                 "VND",
				Channel:                  "web",
				Status:                   "success",
				SourceBalanceBefore:      &sourceBefore,
				SourceBalanceAfter:       &sourceAfter,
				DestinationBalanceBefore: &destinationBefore,
				DestinationBalanceAfter:  &destinationAfter,
			},
		},
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	item, ok, err := bankingTimelineItem(timelineRecorded{StreamID: journal.BankStream, EventID: record.ID, Type: record.Type, Revision: 12, Data: data, Created: at}, accountID, financialID)
	if err != nil || !ok {
		t.Fatalf("bankingTimelineItem() = (%+v, %v, %v), want matched item", item, ok, err)
	}
	if item.Category != "transfer" || item.TransactionID != "tx-1" || item.Details["direction"] != "out" {
		t.Fatalf("timeline item = %+v, want transfer tx-1 out", item)
	}
	balanceAfter, balanceAfterOK := item.Details["balanceAfter"].(*int64)
	if item.Details["balanceBefore"] != sourceBefore || !balanceAfterOK || *balanceAfter != sourceAfter {
		t.Fatalf("timeline balances = %#v, want source balances", item.Details)
	}
}

func TestBankingTimelineItemDoesNotShowSourceOnlyEventForDestination(t *testing.T) {
	accountID := "account-destination"
	sourceID := journal.FinancialAccountID("account-source")
	destinationID := journal.FinancialAccountID(accountID)
	at := time.Date(2026, 10, 30, 7, 30, 0, 0, time.UTC)
	record := journal.Record{
		ID:   "created-event",
		Type: journal.Created,
		At:   at,
		Event: &journal.Event{
			ID:            "created-event",
			TransactionID: "tx-1",
			Type:          journal.Created,
			SchemaVersion: journal.SchemaVersion,
			Sequence:      1,
			OccurredAt:    at,
			Payload:       journal.Payload{SourceAccountID: sourceID, DestinationAccountID: destinationID, Amount: 1, Currency: "VND", Channel: "web", Status: "awaiting_otp"},
		},
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if _, ok, err := bankingTimelineItem(timelineRecorded{EventID: record.ID, Type: record.Type, Data: data, Created: at}, accountID, destinationID); err != nil || ok {
		t.Fatalf("bankingTimelineItem() = (%v, %v), want no destination item", ok, err)
	}
}

func TestTimelineCursorRoundTripStoresBothStreamPositions(t *testing.T) {
	cursor := timelineCursor{BankingRevision: int64Pointer(12), AuditRevision: int64Pointer(7)}
	encoded := encodeTimelineCursor(cursor)
	decoded, err := decodeTimelineCursor(encoded)
	if err != nil {
		t.Fatalf("decode cursor error = %v", err)
	}
	if decoded.BankingRevision == nil || *decoded.BankingRevision != 12 || decoded.AuditRevision == nil || *decoded.AuditRevision != 7 {
		t.Fatalf("decoded cursor = %+v, want banking=12 audit=7", decoded)
	}
}

func TestParseTimelineRangeUsesVietnamTimezone(t *testing.T) {
	from, to, err := parseTimelineRange("2026-10-30", "2026-10-30")
	if err != nil {
		t.Fatalf("parseTimelineRange() error = %v", err)
	}
	if from.Location() != time.UTC || to.Sub(from) != 24*time.Hour {
		t.Fatalf("range = %v to %v, want one UTC day", from, to)
	}
	if from.Hour() != 17 || to.Hour() != 17 {
		t.Fatalf("range hours = %d/%d, want Vietnam midnight in UTC", from.Hour(), to.Hour())
	}
}
