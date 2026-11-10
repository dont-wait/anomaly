package reporting

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/risk"
	"github.com/dont-wait/anomaly/internal/domain/riskassessment"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/dont-wait/anomaly/internal/timeutil"
)

func reportTime(value string) time.Time {
	at, err := time.ParseInLocation("2006-01-02 15:04:05", value, timeutil.VietnamLocation)
	if err != nil {
		panic(err)
	}
	return at.UTC()
}

func testTransfer(id, created, status string, amount int64) Transfer {
	return Transfer{
		ID: id, SourceAccountNo: "source-" + id, SourceName: "Source " + id,
		DestinationAccountNo: "destination-" + id, DestinationName: "Destination " + id,
		CreatedAt: reportTime(created), Status: status, Amount: amount, Fee: amount / 100, Currency: "VND",
	}
}

func testApplied(tx Transfer, score float64, decision string) *risk.Assessment {
	return &risk.Assessment{
		Decision: decision, RiskScore: score, IsAnomalous: decision == risk.DecisionBlock,
		ModelVersion: "model-v1", EvaluatedAt: tx.CreatedAt.Add(time.Second),
	}
}

func testObserved(tx Transfer, score float64, id string) riskassessment.Assessment {
	label, decision := 0, risk.DecisionAllow
	if score >= 0.5 {
		label = 1
	}
	if score > 0.7 {
		decision = risk.DecisionBlock
	}
	return riskassessment.Assessment{
		ID: id, SchemaVersion: 1, TransactionID: tx.ID, EventID: journal.EventID(tx.ID, journal.Created),
		OccurredAt: tx.CreatedAt, ScoredAt: tx.CreatedAt.Add(time.Second), ReceivedAt: tx.CreatedAt.Add(2 * time.Second),
		RiskScore: score, PredictedLabel: label, Threshold: 0.5, Decision: decision,
		ModelVersion: "model-v2", FeatureVersion: "features-v2", Finalized: true, Handled: true,
	}
}

func fixtureBuilder() *Builder {
	return NewBuilder(reportTime("2026-11-08 00:00:00"), reportTime("2026-11-09 00:00:00"),
		reportTime("2026-11-09 00:02:00"), nil)
}

func finishTest(b *Builder) Report { return b.Finish(b.report.AsOf.Add(time.Second)) }

func assertInvariants(t *testing.T, report Report) {
	t.Helper()
	s := report.Summary
	c := s.Created
	if c.TransactionCount != c.SuccessCount+c.FailedCount+c.CancelledCount+c.ExpiredCount+c.PendingCount+c.UnknownCount {
		t.Fatalf("status sum differs: %+v", c)
	}
	if s.Risk.AssessedCount+s.Risk.UnassessedCount != c.TransactionCount ||
		s.Risk.AllowCount+s.Risk.ModelBlockCount+s.Risk.UnknownDecisionCount != s.Risk.AssessedCount ||
		s.Risk.ActualBlockedCount > c.FailedCount {
		t.Fatalf("risk invariant: %+v", s.Risk)
	}
	var created, completed, amount, fee int64
	for i := range 24 {
		hour := report.Hourly.Created[i]
		if hour.Count != hour.SuccessCount+hour.FailedCount+hour.CancelledCount+hour.ExpiredCount+hour.PendingCount+hour.UnknownCount {
			t.Fatalf("hourly status sum: %+v", hour)
		}
		created += hour.Count
		completed += report.Hourly.Completed[i].Count
		amount += report.Hourly.Completed[i].TotalAmount
		fee += report.Hourly.Completed[i].TotalFee
	}
	if created != c.TransactionCount || completed != s.Completed.Count || amount != s.Completed.TotalAmount || fee != s.Completed.TotalFee {
		t.Fatalf("hourly totals differ from summary: %+v", s)
	}
}

func TestReportContractFixture(t *testing.T) {
	b := fixtureBuilder()
	txs := []Transfer{
		testTransfer("T1", "2026-11-08 23:59:00", "success", 100000),
		testTransfer("T2", "2026-11-08 12:00:00", "success", 200000),
		testTransfer("T3", "2026-11-08 13:00:00", "failed", 300000),
		testTransfer("T4", "2026-11-08 14:00:00", "cancelled", 400000),
		testTransfer("T5", "2026-11-08 23:59:30", "awaiting_otp", 500000),
		testTransfer("T6", "2026-11-08 20:00:00", "awaiting_otp", 600000),
		testTransfer("T7", "2026-11-07 23:59:00", "success", 700000),
	}
	txs[0].CompletedAt = timePtr(reportTime("2026-11-09 00:01:00"))
	txs[1].CompletedAt = timePtr(reportTime("2026-11-08 12:01:00"))
	txs[6].CompletedAt = timePtr(reportTime("2026-11-08 00:01:00"))
	txs[0].RiskAssessment = testApplied(txs[0], 0.2, risk.DecisionAllow)
	txs[0].RiskAssessment.EvaluatedAt = reportTime("2026-11-09 00:00:00")
	txs[1].RiskAssessment = testApplied(txs[1], 0.1, risk.DecisionAllow)
	txs[2].RiskAssessment = testApplied(txs[2], 0.9, risk.DecisionBlock)
	txs[2].FailureReason = "anomalous_transfer"
	txs[6].RiskAssessment = testApplied(txs[6], 0.1, risk.DecisionAllow)
	txs[4].OTPExpiresAt = reportTime("2026-11-09 00:04:30")
	txs[5].OTPExpiresAt = reportTime("2026-11-08 20:05:00")
	for i, tx := range txs {
		var candidates []riskassessment.Assessment
		if i == 4 {
			candidates = []riskassessment.Assessment{testObserved(tx, 0, "observed-zero")}
		}
		if i == 0 {
			a := testObserved(tx, 0.2, "late")
			a.ScoredAt = tx.RiskAssessment.EvaluatedAt
			a.ReceivedAt = reportTime("2026-11-09 00:00:30")
			candidates = []riskassessment.Assessment{a}
		}
		if err := b.AddCreated(tx, candidates); err != nil {
			t.Fatal(err)
		}
		if err := b.AddCompleted(tx); err != nil {
			t.Fatal(err)
		}
	}
	r := finishTest(b)
	assertInvariants(t, r)
	c, m, riskSummary := r.Summary.Created, r.Summary.Completed, r.Summary.Risk
	if c.TransactionCount != 6 || c.SuccessCount != 2 || c.FailedCount != 1 || c.CancelledCount != 1 || c.ExpiredCount != 1 || c.PendingCount != 1 {
		t.Fatalf("created = %+v", c)
	}
	if m.Count != 2 || m.TotalAmount != 900000 || m.TotalFee != 9000 || m.AverageAmount == nil || *m.AverageAmount != 450000 {
		t.Fatalf("completed = %+v", m)
	}
	if riskSummary.AssessedCount != 4 || riskSummary.UnassessedCount != 2 || riskSummary.AverageScore == nil ||
		math.Abs(*riskSummary.AverageScore-0.3) > 1e-12 || *riskSummary.Coverage != 4.0/6 || riskSummary.AllowCount != 3 ||
		riskSummary.ModelBlockCount != 1 || riskSummary.ActualBlockedCount != 1 {
		t.Fatalf("risk = %+v", riskSummary)
	}
	if len(r.Transactions) != 6 || r.Transactions[0].TransactionID != "T5" || len(r.DataQuality) != 0 ||
		r.TopRiskSources[0].AccountNo != "source-T3" || r.TopRiskSources[0].Amount != 300000 {
		t.Fatalf("items/top/quality = %+v / %+v / %+v", r.Transactions, r.TopRiskSources, r.DataQuality)
	}
	if r.Hourly.Created[23].Count != 2 || r.Hourly.Completed[0].TotalAmount != 700000 || r.Hourly.Completed[12].TotalAmount != 200000 {
		t.Fatal("incorrect hourly cohorts")
	}
	if r.Previous.Created.TransactionCount != 1 || r.Previous.Created.SuccessCount != 1 || r.Previous.Completed.Count != 0 {
		t.Fatalf("previous = %+v", r.Previous)
	}
}

func TestReportEmptyAndCompletionOnly(t *testing.T) {
	b := fixtureBuilder()
	r := finishTest(b)
	assertInvariants(t, r)
	if r.Summary.Completed.AverageAmount != nil || r.Summary.Risk.AverageScore != nil || r.Summary.Risk.Coverage != nil {
		t.Fatal("empty averages must be null")
	}
	encoded, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(encoded, &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"transactions", "topRiskSources", "topRiskReceivers", "dataQuality"} {
		if values, ok := body[key].([]any); !ok || len(values) != 0 {
			t.Fatalf("%s must be [], got %v", key, body[key])
		}
	}
	tx := testTransfer("old", "2026-11-06 23:59:00", "success", 10000)
	tx.CompletedAt = timePtr(reportTime("2026-11-08 00:00:00"))
	if err := b.AddCompleted(tx); err != nil {
		t.Fatal(err)
	}
	r = finishTest(b)
	assertInvariants(t, r)
	if r.Summary.Created.TransactionCount != 0 || r.Summary.Completed.Count != 1 || r.Summary.Completed.TotalAmount != 10000 {
		t.Fatalf("completion-only day: %+v", r.Summary)
	}
}

func TestReportStatusesAndBoundaries(t *testing.T) {
	asOf := reportTime("2026-11-09 00:02:00")
	tests := []struct {
		status, want, issue string
		expiry              time.Time
	}{
		{"awaiting_otp", "expired", "", asOf},
		{"awaiting_otp", "pending", "", asOf.Add(time.Millisecond)},
		{"awaiting_otp", "pending", "MISSING_OTP_EXPIRY", time.Time{}},
		{"cancelled", "cancelled", "", asOf.Add(-time.Hour)},
		{"success", "success", "", asOf.Add(-time.Hour)},
		{"failed", "failed", "", asOf.Add(-time.Hour)},
		{"unexpected", "unknown", "UNKNOWN_STATUS", time.Time{}},
	}
	for _, tc := range tests {
		t.Run(tc.status+tc.want+tc.issue, func(t *testing.T) {
			tx := testTransfer("id", "2026-11-08 12:00:00", tc.status, 10000)
			tx.OTPExpiresAt = tc.expiry
			status, _, issue := effectiveStatus(tx, asOf)
			if status != tc.want || issue != tc.issue {
				t.Fatalf("got %s/%s, want %s/%s", status, issue, tc.want, tc.issue)
			}
		})
	}
	b := fixtureBuilder()
	for _, created := range []string{"2026-11-08 00:00:00", "2026-11-09 00:00:00", "2026-11-09 00:03:00"} {
		if err := b.AddCreated(testTransfer(created, created, "cancelled", 10000), nil); err != nil {
			t.Fatal(err)
		}
	}
	r := finishTest(b)
	assertInvariants(t, r)
	if r.Summary.Created.TransactionCount != 1 || r.Hourly.Created[0].Count != 1 {
		t.Fatal("day boundaries are not half-open")
	}
}

func TestReportAppliedRiskCannotBeReplacedOrEnrichedByUnprovenAssessment(t *testing.T) {
	tx := testTransfer("id", "2026-11-08 12:00:00", "failed", 10000)
	tx.FailureReason = "anomalous_transfer"
	tx.RiskAssessment = testApplied(tx, 0.9, risk.DecisionBlock)
	observed := testObserved(tx, 0.1, "newer")
	observed.ScoredAt = observed.ScoredAt.Add(time.Hour)
	observed.ReceivedAt = observed.ScoredAt
	b := fixtureBuilder()
	if err := b.AddCreated(tx, []riskassessment.Assessment{observed}); err != nil {
		t.Fatal(err)
	}
	r := finishTest(b)
	item := r.Transactions[0]
	if item.AssessmentSource != "applied" || item.SelectedAssessment.RiskScore != 0.9 ||
		*item.SelectedAssessment.Decision != risk.DecisionBlock || item.SelectedAssessment.Threshold != nil ||
		item.SelectedAssessment.AssessmentID != nil || item.SelectedAssessment.ReceivedAt != nil || item.LatestAssessment.RiskScore != 0.1 {
		t.Fatalf("mixed applied and observed metadata: %+v", item)
	}
	assertInvariants(t, r)
	// Legacy decision is unknown; do not derive it from isAnomalous/score.
	tx.RiskAssessment.Decision = ""
	b = fixtureBuilder()
	if err := b.AddCreated(tx, nil); err != nil {
		t.Fatal(err)
	}
	r = finishTest(b)
	if r.Summary.Risk.UnknownDecisionCount != 1 || r.Summary.Risk.ActualBlockedCount != 1 || r.Summary.Risk.ModelBlockCount != 0 {
		t.Fatalf("legacy risk: %+v", r.Summary.Risk)
	}
}

func TestReportMissingAndInvalidAssessmentNeverBecomesAllow(t *testing.T) {
	for _, mode := range []string{"missing", "nan", "infinite", "range", "future", "invalid-applied-with-observed"} {
		t.Run(mode, func(t *testing.T) {
			tx := testTransfer("id", "2026-11-08 12:00:00", "cancelled", 10000)
			var candidates []riskassessment.Assessment
			if mode != "missing" {
				tx.RiskAssessment = testApplied(tx, 0.1, risk.DecisionAllow)
				switch mode {
				case "nan", "invalid-applied-with-observed":
					tx.RiskAssessment.RiskScore = math.NaN()
				case "infinite":
					tx.RiskAssessment.RiskScore = math.Inf(1)
				case "range":
					tx.RiskAssessment.RiskScore = 1.1
				case "future":
					tx.RiskAssessment.EvaluatedAt = reportTime("2026-11-10 00:00:00")
				}
			}
			if mode == "invalid-applied-with-observed" {
				candidates = []riskassessment.Assessment{testObserved(tx, 0.1, "valid-observed")}
			}
			b := fixtureBuilder()
			if err := b.AddCreated(tx, candidates); err != nil {
				t.Fatal(err)
			}
			r := finishTest(b)
			if r.Transactions[0].SelectedAssessment != nil || r.Transactions[0].AssessmentSource != "none" ||
				r.Summary.Risk.AssessedCount != 0 || r.Summary.Risk.AllowCount != 0 || *r.Summary.Risk.Coverage != 0 {
				t.Fatalf("fabricated assessment: %+v", r)
			}
			if _, err := json.Marshal(r); err != nil {
				t.Fatal("invalid scores leaked into response:", err)
			}
			assertInvariants(t, r)
		})
	}
}

func TestReportLatestAssessmentProvenanceTiesAndMoreThan50(t *testing.T) {
	b := fixtureBuilder()
	for i := range 61 {
		tx := testTransfer(fmt.Sprintf("tx-%02d", i), "2026-11-08 12:00:00", "awaiting_otp", 10000)
		tx.OTPExpiresAt = b.report.AsOf.Add(time.Minute)
		older, newer := testObserved(tx, 0.1, "a"), testObserved(tx, 0.9, "z")
		wrongEvent := testObserved(tx, 0.2, "wrong-event")
		wrongEvent.EventID = "unrelated-event"
		future := testObserved(tx, 0.2, "future")
		future.ReceivedAt = b.report.AsOf.Add(time.Second)
		if err := b.AddCreated(tx, []riskassessment.Assessment{newer, older, wrongEvent, future, newer}); err != nil {
			t.Fatal(err)
		}
	}
	r := finishTest(b)
	assertInvariants(t, r)
	if len(r.Transactions) != 61 || r.Summary.Risk.ModelBlockCount != 61 || r.Summary.Risk.ActualBlockedCount != 0 || r.Summary.Created.PendingCount != 61 {
		t.Fatalf("assessment cap/double counting/status mutation: %+v", r.Summary)
	}
	for i, item := range r.Transactions {
		if *item.LatestAssessment.AssessmentID != "z" || item.TransactionID != fmt.Sprintf("tx-%02d", i) {
			t.Fatalf("nondeterministic selection/sort: %+v", item)
		}
	}
	if len(r.TopRiskSources) != 5 || r.TopRiskSources[0].AccountNo != "source-tx-00" {
		t.Fatalf("top tie ordering: %+v", r.TopRiskSources)
	}
}

func TestReportCompletionFallbackAndGlobalQuality(t *testing.T) {
	b := fixtureBuilder()
	txs := []Transfer{
		testTransfer("fallback", "2026-10-01 12:00:00", "success", 10000),
		testTransfer("missing", "2026-10-01 12:00:00", "success", 10000),
		testTransfer("invalid", "2026-10-01 12:00:00", "success", 10000),
	}
	txs[0].PostedAt = reportTime("2026-11-08 00:00:00")
	txs[2].CompletedAt = timePtr(reportTime("2026-09-01 00:00:00"))
	txs[2].PostedAt = reportTime("2026-11-08 00:00:00")
	for _, tx := range txs {
		if err := b.AddCompleted(tx); err != nil {
			t.Fatal(err)
		}
	}
	r := finishTest(b)
	if r.Summary.Completed.Count != 1 || r.Summary.Completed.TotalAmount != 10000 || len(r.DataQuality) != 2 {
		t.Fatalf("completion validation: %+v", r)
	}
	for _, issue := range r.DataQuality {
		if issue.Scope != "global" || issue.Date != nil || issue.Count != 1 {
			t.Fatalf("wrong completion quality scope: %+v", issue)
		}
	}
	assertInvariants(t, r)
}

func TestReportInvalidMoneyAndOverflow(t *testing.T) {
	for _, mode := range []string{"currency", "negative", "unsafe"} {
		t.Run(mode, func(t *testing.T) {
			tx := testTransfer("id", "2026-11-08 12:00:00", "success", 10000)
			tx.CompletedAt = timePtr(tx.CreatedAt.Add(time.Minute))
			switch mode {
			case "currency":
				tx.Currency = "USD"
			case "negative":
				tx.Amount = -1
			case "unsafe":
				tx.Amount = MaxSafeMoney + 1
			}
			b := fixtureBuilder()
			if err := b.AddCreated(tx, nil); err != nil {
				t.Fatal(err)
			}
			if err := b.AddCompleted(tx); err != nil {
				t.Fatal(err)
			}
			r := finishTest(b)
			if r.Transactions[0].Amount != nil || r.Transactions[0].Fee != nil || r.Summary.Completed.Count != 0 || len(r.DataQuality) != 2 {
				t.Fatalf("invalid money was counted/rounded: %+v", r)
			}
			assertInvariants(t, r)
		})
	}
	b := fixtureBuilder()
	tx := testTransfer("large", "2026-11-08 12:00:00", "success", MaxSafeMoney)
	tx.CompletedAt = timePtr(tx.CreatedAt.Add(time.Second))
	if err := b.AddCompleted(tx); err != nil {
		t.Fatal(err)
	}
	tx.Amount = 1
	if err := b.AddCompleted(tx); !errors.Is(err, ErrUnsafeMoneyTotal) {
		t.Fatalf("unsafe aggregate did not fail: %v", err)
	}
}

func TestReportRiskAndAccountOrderingIndependentOfInput(t *testing.T) {
	build := func(reverse bool) Report {
		b := fixtureBuilder()
		for i := range 8 {
			if reverse {
				i = 7 - i
			}
			tx := testTransfer(fmt.Sprint(i), "2026-11-08 12:00:00", "failed", 10000)
			tx.RiskAssessment = testApplied(tx, 0.9, risk.DecisionBlock)
			if err := b.AddCreated(tx, nil); err != nil {
				t.Fatal(err)
			}
		}
		return finishTest(b)
	}
	if !reflect.DeepEqual(build(false), build(true)) {
		t.Fatal("report ordering depends on map/database input order")
	}
}

func TestReportQualityDatesUnknownStatusAndMissingRiskEvidence(t *testing.T) {
	b := fixtureBuilder()
	for _, date := range []string{"2026-11-07 12:00:00", "2026-11-08 12:00:00"} {
		if err := b.AddCreated(testTransfer(date, date, "awaiting_otp", 10000), nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.AddCreated(testTransfer("unknown", "2026-11-08 13:00:00", "unrecognized", 10000), nil); err != nil {
		t.Fatal(err)
	}
	blocked := testTransfer("blocked-without-assessment", "2026-11-08 14:00:00", "failed", 10000)
	blocked.FailureReason = "anomalous_transfer"
	if err := b.AddCreated(blocked, nil); err != nil {
		t.Fatal(err)
	}
	r := finishTest(b)
	assertInvariants(t, r)
	if r.Summary.Created.UnknownCount != 1 || r.Summary.Risk.ActualBlockedCount != 1 ||
		r.Summary.Risk.AssessedCount != 0 || len(r.DataQuality) != 3 || r.Previous.Created.PendingCount != 1 {
		t.Fatalf("quality/status/risk evidence: %+v", r)
	}
	if *r.DataQuality[0].Date != "2026-11-07" || *r.DataQuality[1].Date != "2026-11-08" {
		t.Fatalf("quality dates not separated: %+v", r.DataQuality)
	}
	if again := finishTest(b); !reflect.DeepEqual(r, again) {
		t.Fatal("finishing a report twice duplicated quality warnings")
	}
}

func TestReportModelLabelDoesNotOverrideStoredBusinessDecision(t *testing.T) {
	tx := testTransfer("warning", "2026-11-08 12:00:00", "awaiting_otp", 10000)
	tx.OTPExpiresAt = reportTime("2026-11-09 01:00:00")
	observed := testObserved(tx, 0.6, "warning") // model label=1, business decision=ALLOW
	b := fixtureBuilder()
	if err := b.AddCreated(tx, []riskassessment.Assessment{observed}); err != nil {
		t.Fatal(err)
	}
	r := finishTest(b)
	if r.Summary.Risk.AllowCount != 1 || r.Summary.Risk.ModelBlockCount != 0 || r.Summary.Risk.ActualBlockedCount != 0 {
		t.Fatalf("model label treated as business BLOCK: %+v", r.Summary.Risk)
	}
	assertInvariants(t, r)
}
