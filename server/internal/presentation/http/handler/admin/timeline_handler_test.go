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

func TestTimelinePaginationUsesStableCursor(t *testing.T) {
	when := time.Date(2026, 10, 30, 7, 0, 0, 0, time.UTC)
	items := []timelineEvent{
		{Item: timelineItemDTO{EventID: "3", OccurredAt: when, SourceStream: "banking", SourceRevision: 3}, Key: "banking:00000000000000000003:3"},
		{Item: timelineItemDTO{EventID: "2", OccurredAt: when, SourceStream: "banking", SourceRevision: 2}, Key: "banking:00000000000000000002:2"},
		{Item: timelineItemDTO{EventID: "1", OccurredAt: when.Add(-time.Minute), SourceStream: "banking", SourceRevision: 1}, Key: "banking:00000000000000000001:1"},
	}
	page, next, more := paginateTimeline(items, timelineCursor{}, 2)
	if len(page) != 2 || next == "" || !more {
		t.Fatalf("first page = %d/%q/%v, want two items, cursor, more", len(page), next, more)
	}
	cursor, err := decodeTimelineCursor(next)
	if err != nil {
		t.Fatalf("decode cursor error = %v", err)
	}
	page, next, more = paginateTimeline(items, cursor, 2)
	if len(page) != 1 || next != "" || more || page[0].EventID != "1" {
		t.Fatalf("second page = %#v/%q/%v, want event 1 and end", page, next, more)
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
