package admin

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/timeline"
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

func TestBankingTimelineItemIncludesLoanDisbursement(t *testing.T) {
	at := time.Date(2026, 10, 30, 7, 30, 0, 0, time.UTC)
	before, after := int64(1_000_000), int64(11_000_000)
	record := journal.Record{
		ID:            "loan-credit-event",
		Type:          journal.LoanDisbursementCredited,
		At:            at,
		AccountID:     "account-1",
		LoanID:        "loan-1",
		LoanAmount:    10_000_000,
		BalanceBefore: &before,
		BalanceAfter:  &after,
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	item, ok, err := bankingTimelineItem(timelineRecorded{EventID: record.ID, Type: record.Type, Revision: 4, Data: data, Created: at}, "account-1", journal.FinancialAccountID("account-1"))
	if err != nil || !ok {
		t.Fatalf("bankingTimelineItem() = (%+v, %v, %v), want loan item", item, ok, err)
	}
	if item.Category != "loan" || item.LoanID != "loan-1" || item.Details["amount"] != int64(10_000_000) {
		t.Fatalf("loan timeline item = %+v", item)
	}
}

func TestLoanTimelineItemMapsMaskedLoanEvent(t *testing.T) {
	when := time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC)
	item := loanTimelineItem(timeline.LoanEvent{
		EventID:        "loan-event",
		EventType:      "LoanInstallmentPaid",
		LoanID:         "loan-1",
		AccountID:      "account-1",
		SourceStream:   "loan-application-loan-1",
		SourceRevision: 5,
		OccurredAt:     when,
		Summary:        "Đã trả một kỳ khoản vay",
		Details:        map[string]any{"installmentNo": 1},
	})
	if item.Category != "loan" || item.LoanID != "loan-1" || item.Details["installmentNo"] != 1 || !item.OccurredAt.Equal(when) {
		t.Fatalf("loan timeline item = %+v", item)
	}
}

func TestTimelineCursorRoundTripStoresBothStreamPositions(t *testing.T) {
	cursor := timelineCursor{Version: timelineCursorV3, AccountID: "account-1", Category: "all", From: "2026-10-01", To: "2026-10-30", BankingRevision: int64Pointer(12), AuditRevision: int64Pointer(7)}
	encoded := encodeTimelineCursor(cursor)
	decoded, err := decodeTimelineCursor(encoded)
	if err != nil {
		t.Fatalf("decode cursor error = %v", err)
	}
	if decoded.Version != timelineCursorV3 || decoded.AccountID != "account-1" || decoded.Category != "all" || decoded.From != "2026-10-01" || decoded.To != "2026-10-30" || decoded.BankingRevision == nil || *decoded.BankingRevision != 12 || decoded.AuditRevision == nil || *decoded.AuditRevision != 7 {
		t.Fatalf("decoded cursor = %+v, want banking=12 audit=7", decoded)
	}
}

func TestSelectTimelineCandidateMergesBankingAndAuditByOccurrence(t *testing.T) {
	banking := &timelineCandidate{item: timelineItemDTO{EventID: "banking-event", OccurredAt: time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC), SourceStream: journal.BankStream}}
	audit := &timelineCandidate{item: timelineItemDTO{EventID: "audit-event", OccurredAt: time.Date(2026, 10, 30, 7, 0, 0, 0, time.UTC), SourceStream: auditAccountPrefix + "account-1"}}

	if selected := selectTimelineCandidate(banking, audit); selected != banking {
		t.Fatalf("selected candidate = %#v, want banking candidate", selected)
	}
	if selected := selectTimelineCandidate(audit, banking); selected != banking {
		t.Fatalf("selected candidate = %#v, want banking candidate regardless of argument order", selected)
	}
}

func TestSelectTimelineCandidateUsesStableTieBreak(t *testing.T) {
	when := time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC)
	banking := &timelineCandidate{item: timelineItemDTO{EventID: "banking-event", OccurredAt: when, SourceStream: journal.BankStream, SourceRevision: 2}}
	audit := &timelineCandidate{item: timelineItemDTO{EventID: "audit-event", OccurredAt: when, SourceStream: auditAccountPrefix + "account-1", SourceRevision: 1}}

	if selected := selectTimelineCandidate(banking, audit); selected != banking {
		t.Fatalf("selected candidate = %#v, want stable key ordering", selected)
	}
}

func TestTimelineMatchesFiltersCategoryAndInclusiveDateRange(t *testing.T) {
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		item     timelineItemDTO
		category string
		want     bool
	}{
		{
			name:     "matches category and start boundary",
			item:     timelineItemDTO{Category: "otp", OccurredAt: from},
			category: "otp",
			want:     true,
		},
		{
			name:     "rejects different category",
			item:     timelineItemDTO{Category: "login", OccurredAt: from},
			category: "otp",
			want:     false,
		},
		{
			name:     "all category accepts matching event",
			item:     timelineItemDTO{Category: "transfer", OccurredAt: from.Add(24 * time.Hour)},
			category: "all",
			want:     true,
		},
		{
			name:     "rejects end boundary",
			item:     timelineItemDTO{Category: "otp", OccurredAt: to},
			category: "otp",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := timelineMatches(tt.item, from, to, tt.category); got != tt.want {
				t.Fatalf("timelineMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTimelineCursorRejectsDifferentQuery(t *testing.T) {
	cursor := timelineCursor{Version: timelineCursorV3, AccountID: "account-1", Category: "all", From: "2026-10-01", To: "2026-10-30", BankingRevision: int64Pointer(12), AuditRevision: int64Pointer(7)}
	if err := validateTimelineCursor(cursor, "account-2", "all", "2026-10-01", "2026-10-30"); err == nil {
		t.Fatal("validateTimelineCursor() accepted a different account")
	}
	if err := validateTimelineCursor(cursor, "account-1", "otp", "2026-10-01", "2026-10-30"); err == nil {
		t.Fatal("validateTimelineCursor() accepted a different category")
	}
	if err := validateTimelineCursor(cursor, "account-1", "all", "2026-09-01", "2026-09-30"); err == nil {
		t.Fatal("validateTimelineCursor() accepted a different date range")
	}
}

func TestDecodeTimelineCursorRejectsLegacyOrPartialCursor(t *testing.T) {
	legacy := encodeTimelineCursor(timelineCursor{BankingRevision: int64Pointer(12), AuditRevision: int64Pointer(7)})
	if _, err := decodeTimelineCursor(legacy); err == nil {
		t.Fatal("decodeTimelineCursor() accepted a legacy cursor")
	}
	partial := encodeTimelineCursor(timelineCursor{Version: timelineCursorV3, AccountID: "account-1", Category: "all", BankingRevision: int64Pointer(12)})
	if _, err := decodeTimelineCursor(partial); err == nil {
		t.Fatal("decodeTimelineCursor() accepted a partial cursor")
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
