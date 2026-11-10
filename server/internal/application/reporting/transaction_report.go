// Package reporting defines the admin transaction report independently of its
// storage adapter. See docs/admin-transaction-report-contract.md.
package reporting

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/risk"
	"github.com/dont-wait/anomaly/internal/domain/riskassessment"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/dont-wait/anomaly/internal/timeutil"
)

const MaxSafeMoney int64 = 1<<53 - 1

var ErrUnsafeMoneyTotal = errors.New("report total exceeds safe integer precision")

type Reader interface {
	TransactionReport(context.Context, time.Time, time.Time) (Report, error)
}

type StatusCounts struct {
	SuccessCount   int64 `json:"successCount"`
	FailedCount    int64 `json:"failedCount"`
	CancelledCount int64 `json:"cancelledCount"`
	ExpiredCount   int64 `json:"expiredCount"`
	PendingCount   int64 `json:"pendingCount"`
	UnknownCount   int64 `json:"unknownCount"`
}

func (c *StatusCounts) add(status string) {
	switch status {
	case "success":
		c.SuccessCount++
	case "failed":
		c.FailedCount++
	case "cancelled":
		c.CancelledCount++
	case "expired":
		c.ExpiredCount++
	case "pending":
		c.PendingCount++
	default:
		c.UnknownCount++
	}
}

type CreatedSummary struct {
	TransactionCount int64 `json:"transactionCount"`
	StatusCounts
}

type CompletedSummary struct {
	Count         int64    `json:"count"`
	TotalAmount   int64    `json:"totalAmount"`
	TotalFee      int64    `json:"totalFee"`
	AverageAmount *float64 `json:"averageAmount"`
}

type RiskSummary struct {
	AssessedCount        int64    `json:"assessedCount"`
	UnassessedCount      int64    `json:"unassessedCount"`
	AverageScore         *float64 `json:"averageScore"`
	Coverage             *float64 `json:"coverage"`
	AllowCount           int64    `json:"allowCount"`
	ModelBlockCount      int64    `json:"modelBlockCount"`
	UnknownDecisionCount int64    `json:"unknownDecisionCount"`
	ActualBlockedCount   int64    `json:"actualBlockedCount"`
}

type Summary struct {
	Created   CreatedSummary   `json:"created"`
	Completed CompletedSummary `json:"completed"`
	Risk      RiskSummary      `json:"risk"`
}

type CreatedHour struct {
	Hour  string `json:"hour"`
	Count int64  `json:"count"`
	StatusCounts
}

type CompletedHour struct {
	Hour        string `json:"hour"`
	Count       int64  `json:"count"`
	TotalAmount int64  `json:"totalAmount"`
	TotalFee    int64  `json:"totalFee"`
}

type Hourly struct {
	Created   []CreatedHour   `json:"created"`
	Completed []CompletedHour `json:"completed"`
}

type Assessment struct {
	AssessmentID   *string    `json:"assessmentId"`
	Decision       *string    `json:"decision"`
	RiskScore      float64    `json:"riskScore"`
	Threshold      *float64   `json:"threshold"`
	ModelVersion   string     `json:"modelVersion"`
	FeatureVersion *string    `json:"featureVersion"`
	EvaluatedAt    time.Time  `json:"evaluatedAt"`
	ReceivedAt     *time.Time `json:"receivedAt"`
}

type Item struct {
	TransactionID        string      `json:"transactionId"`
	SourceAccountNo      string      `json:"sourceAccountNo"`
	SourceAccountName    string      `json:"sourceAccountName"`
	DestinationAccountNo string      `json:"destinationAccountNo"`
	DestinationName      string      `json:"destinationName"`
	Amount               *int64      `json:"amount"`
	Fee                  *int64      `json:"fee"`
	Currency             string      `json:"currency"`
	CanonicalStatus      string      `json:"canonicalStatus"`
	EffectiveStatus      string      `json:"effectiveStatus"`
	StatusAt             *time.Time  `json:"statusAt"`
	FailureReason        *string     `json:"failureReason"`
	CreatedAt            time.Time   `json:"createdAt"`
	CompletedAt          *time.Time  `json:"completedAt"`
	CompletionTimeSource *string     `json:"completionTimeSource"`
	OTPExpiresAt         *time.Time  `json:"otpExpiresAt"`
	AppliedRisk          *Assessment `json:"appliedRisk"`
	LatestAssessment     *Assessment `json:"latestAssessment"`
	SelectedAssessment   *Assessment `json:"selectedAssessment"`
	AssessmentSource     string      `json:"assessmentSource"`
}

type RiskAccount struct {
	AccountNo string `json:"accountNo"`
	Name      string `json:"name"`
	Count     int64  `json:"count"`
	Amount    int64  `json:"amount"`
}

type QualityIssue struct {
	Code  string  `json:"code"`
	Scope string  `json:"scope"`
	Date  *string `json:"date"`
	Count int64   `json:"count"`
}

type Projection struct {
	Status             string  `json:"status"`
	CheckpointRevision *string `json:"checkpointRevision"`
}

type Report struct {
	ReportVersion    int            `json:"reportVersion"`
	Date             string         `json:"date"`
	PreviousDate     string         `json:"previousDate"`
	Timezone         string         `json:"timezone"`
	Currency         string         `json:"currency"`
	From             time.Time      `json:"from"`
	To               time.Time      `json:"to"`
	AsOf             time.Time      `json:"asOf"`
	GeneratedAt      time.Time      `json:"generatedAt"`
	Projection       Projection     `json:"projection"`
	Summary          Summary        `json:"summary"`
	Previous         Summary        `json:"previous"`
	Hourly           Hourly         `json:"hourly"`
	Transactions     []Item         `json:"transactions"`
	TopRiskSources   []RiskAccount  `json:"topRiskSources"`
	TopRiskReceivers []RiskAccount  `json:"topRiskReceivers"`
	DataQuality      []QualityIssue `json:"dataQuality"`
}

// Transfer is a canonical transaction projection, not a debit/credit feed row.
type Transfer struct {
	ID, SourceAccountNo, SourceName, DestinationAccountNo, DestinationName string
	Amount, Fee                                                            int64
	Currency, Status, FailureReason                                        string
	CreatedAt, PostedAt, OTPExpiresAt                                      time.Time
	CompletedAt, FailedAt, CancelledAt                                     *time.Time
	RiskAssessment                                                         *risk.Assessment
}

type period struct {
	summary Summary
	score   float64
}

// Builder receives created and completed projections separately so the adapter
// can stream global completion-time validation without retaining all history.
type Builder struct {
	report             Report
	current, previous  period
	sources, receivers map[string]RiskAccount
	issues             map[string]QualityIssue
}

func NewBuilder(from, to, asOf time.Time, checkpoint *string) *Builder {
	b := &Builder{
		report: Report{
			ReportVersion: 2, Date: from.In(timeutil.VietnamLocation).Format(time.DateOnly),
			PreviousDate: from.In(timeutil.VietnamLocation).AddDate(0, 0, -1).Format(time.DateOnly),
			Timezone:     "Asia/Ho_Chi_Minh", Currency: "VND", From: from.UTC(), To: to.UTC(), AsOf: asOf.UTC(),
			Projection:   Projection{Status: "unknown", CheckpointRevision: checkpoint},
			Transactions: []Item{}, TopRiskSources: []RiskAccount{}, TopRiskReceivers: []RiskAccount{}, DataQuality: []QualityIssue{},
			Hourly: Hourly{Created: make([]CreatedHour, 24), Completed: make([]CompletedHour, 24)},
		},
		sources: map[string]RiskAccount{}, receivers: map[string]RiskAccount{}, issues: map[string]QualityIssue{},
	}
	for hour := range 24 {
		label := fmt.Sprintf("%02d:00", hour)
		b.report.Hourly.Created[hour].Hour = label
		b.report.Hourly.Completed[hour].Hour = label
	}
	return b
}

func (b *Builder) periodAt(at time.Time) (*period, bool) {
	if at.IsZero() || at.After(b.report.AsOf) || !at.Before(b.report.To) {
		return nil, false
	}
	if !at.Before(b.report.From) {
		return &b.current, true
	}
	if !at.Before(b.report.From.In(timeutil.VietnamLocation).AddDate(0, 0, -1)) {
		return &b.previous, false
	}
	return nil, false
}

func (b *Builder) issue(code, scope string, date *string) {
	key := code + ":" + scope
	if date != nil {
		key += ":" + *date
	}
	item := b.issues[key]
	item.Code, item.Scope, item.Date = code, scope, date
	item.Count++
	b.issues[key] = item
}

func validMoney(tx Transfer) bool {
	return tx.Currency == "VND" && tx.Amount > 0 && tx.Amount <= MaxSafeMoney && tx.Fee >= 0 && tx.Fee <= MaxSafeMoney
}

func addMoney(total *int64, value int64) error {
	if value > MaxSafeMoney-*total {
		return ErrUnsafeMoneyTotal
	}
	*total += value
	return nil
}

func completion(tx Transfer) (*time.Time, *string, string) {
	if tx.Status != "success" {
		return nil, nil, ""
	}
	at, source := tx.CompletedAt, "completed_at"
	if at == nil || at.IsZero() {
		at, source = &tx.PostedAt, "posted_at"
	}
	if at.IsZero() {
		return nil, nil, "MISSING_COMPLETION_TIME"
	}
	if at.Before(tx.CreatedAt) {
		return nil, nil, "INVALID_COMPLETION_TIME"
	}
	return timePtr(*at), &source, ""
}

func effectiveStatus(tx Transfer, asOf time.Time) (string, *time.Time, string) {
	switch tx.Status {
	case "success":
		at, _, _ := completion(tx)
		return "success", at, ""
	case "failed":
		return "failed", optionalTime(tx.FailedAt), ""
	case "cancelled":
		return "cancelled", optionalTime(tx.CancelledAt), ""
	case "awaiting_otp":
		if tx.OTPExpiresAt.IsZero() {
			return "pending", timePtr(tx.CreatedAt), "MISSING_OTP_EXPIRY"
		}
		if !tx.OTPExpiresAt.After(asOf) {
			return "expired", timePtr(tx.OTPExpiresAt), ""
		}
		return "pending", timePtr(tx.CreatedAt), ""
	default:
		return "unknown", nil, "UNKNOWN_STATUS"
	}
}

// LatestAssessment validates provenance and time, then selects deterministically.
// It does not reinterpret model labels/thresholds as a business decision.
func LatestAssessment(tx Transfer, candidates []riskassessment.Assessment, asOf time.Time) (*Assessment, bool) {
	var latest *riskassessment.Assessment
	invalid := false
	for i := range candidates {
		a := &candidates[i]
		if a.ReceivedAt.After(asOf) || a.ScoredAt.After(asOf) {
			continue
		}
		if a.TransactionID != tx.ID || a.EventID != journal.EventID(tx.ID, journal.Created) {
			invalid = true
			continue
		}
		if strings.TrimSpace(a.ID) == "" || a.SchemaVersion != 1 || a.OccurredAt.IsZero() || a.OccurredAt.After(asOf) ||
			a.ReceivedAt.IsZero() || a.ScoredAt.IsZero() || a.ScoredAt.Before(tx.CreatedAt) ||
			!validScore(a.RiskScore) || strings.TrimSpace(a.ModelVersion) == "" || strings.TrimSpace(a.FeatureVersion) == "" ||
			!validScore(a.Threshold) || (a.Decision != risk.DecisionAllow && a.Decision != risk.DecisionBlock) {
			invalid = true
			continue
		}
		expectedLabel := 0
		if a.RiskScore >= a.Threshold {
			expectedLabel = 1
		}
		if a.PredictedLabel != expectedLabel {
			invalid = true
			continue
		}
		if latest == nil || a.ScoredAt.After(latest.ScoredAt) ||
			(a.ScoredAt.Equal(latest.ScoredAt) && (a.ReceivedAt.After(latest.ReceivedAt) ||
				(a.ReceivedAt.Equal(latest.ReceivedAt) && a.ID > latest.ID))) {
			latest = a
		}
	}
	if latest == nil {
		return nil, invalid
	}
	return &Assessment{
		AssessmentID: stringPtr(latest.ID), Decision: stringPtr(latest.Decision), RiskScore: latest.RiskScore,
		Threshold: &latest.Threshold, ModelVersion: latest.ModelVersion, FeatureVersion: stringPtr(latest.FeatureVersion),
		EvaluatedAt: latest.ScoredAt.UTC(), ReceivedAt: timePtr(latest.ReceivedAt),
	}, invalid
}

func appliedRisk(tx Transfer, asOf time.Time) (*Assessment, bool) {
	a := tx.RiskAssessment
	if a == nil {
		return nil, false
	}
	if !validScore(a.RiskScore) || strings.TrimSpace(a.ModelVersion) == "" || a.EvaluatedAt.IsZero() ||
		a.EvaluatedAt.After(asOf) || a.EvaluatedAt.Before(tx.CreatedAt) ||
		(a.Decision != "" && a.Decision != risk.DecisionAllow && a.Decision != risk.DecisionBlock) {
		return nil, true
	}
	return &Assessment{
		Decision: stringPtr(a.Decision), RiskScore: a.RiskScore,
		ModelVersion: a.ModelVersion, EvaluatedAt: a.EvaluatedAt.UTC(),
	}, false
}

func validScore(score float64) bool {
	return !math.IsNaN(score) && !math.IsInf(score, 0) && score >= 0 && score <= 1
}

func (b *Builder) AddCreated(tx Transfer, candidates []riskassessment.Assessment) error {
	p, current := b.periodAt(tx.CreatedAt)
	if p == nil {
		return nil
	}
	date := tx.CreatedAt.In(timeutil.VietnamLocation).Format(time.DateOnly)
	status, statusAt, statusIssue := effectiveStatus(tx, b.report.AsOf)
	if statusIssue != "" {
		b.issue(statusIssue, "created", &date)
	}
	p.summary.Created.TransactionCount++
	p.summary.Created.StatusCounts.add(status)
	latest, invalidObserved := LatestAssessment(tx, candidates, b.report.AsOf)
	applied, invalidApplied := appliedRisk(tx, b.report.AsOf)
	if invalidObserved || invalidApplied {
		b.issue("INVALID_ASSESSMENT", "created", &date)
	}
	selected, source := applied, "applied"
	if tx.RiskAssessment == nil {
		selected, source = latest, "observed"
	}
	if selected == nil {
		source = "none"
		p.summary.Risk.UnassessedCount++
	} else {
		p.summary.Risk.AssessedCount++
		p.score += selected.RiskScore
		switch {
		case selected.Decision == nil:
			p.summary.Risk.UnknownDecisionCount++
		case *selected.Decision == risk.DecisionBlock:
			p.summary.Risk.ModelBlockCount++
		default:
			p.summary.Risk.AllowCount++
		}
	}
	if tx.Status == "failed" && tx.FailureReason == "anomalous_transfer" {
		p.summary.Risk.ActualBlockedCount++
	}
	if !validMoney(tx) {
		b.issue("INVALID_MONEY", "created", &date)
	}
	if !current {
		return nil
	}
	hour := &b.report.Hourly.Created[tx.CreatedAt.In(timeutil.VietnamLocation).Hour()]
	hour.Count++
	hour.StatusCounts.add(status)
	completedAt, completionSource, _ := completion(tx)
	item := Item{
		TransactionID: tx.ID, SourceAccountNo: tx.SourceAccountNo, SourceAccountName: tx.SourceName,
		DestinationAccountNo: tx.DestinationAccountNo, DestinationName: tx.DestinationName, Currency: tx.Currency,
		CanonicalStatus: tx.Status, EffectiveStatus: status, StatusAt: statusAt, FailureReason: stringPtr(tx.FailureReason),
		CreatedAt: tx.CreatedAt.UTC(), CompletedAt: completedAt, CompletionTimeSource: completionSource,
		OTPExpiresAt: timePtr(tx.OTPExpiresAt), AppliedRisk: applied, LatestAssessment: latest,
		SelectedAssessment: selected, AssessmentSource: source,
	}
	if validMoney(tx) {
		item.Amount, item.Fee = &tx.Amount, &tx.Fee
	}
	b.report.Transactions = append(b.report.Transactions, item)
	if selected != nil && selected.Decision != nil && *selected.Decision == risk.DecisionBlock {
		for _, side := range []struct {
			values map[string]RiskAccount
			no     string
			name   string
		}{
			{b.sources, tx.SourceAccountNo, tx.SourceName},
			{b.receivers, tx.DestinationAccountNo, tx.DestinationName},
		} {
			if side.no == "" {
				b.issue("MISSING_RISK_ACCOUNT", "created", &date)
				continue
			}
			account := side.values[side.no]
			account.AccountNo = side.no
			// Deterministic even if account names changed between transfers.
			if account.Name == "" || (side.name != "" && side.name < account.Name) {
				account.Name = side.name
			}
			account.Count++
			if validMoney(tx) {
				if err := addMoney(&account.Amount, tx.Amount); err != nil {
					return err
				}
			}
			side.values[side.no] = account
		}
	}
	return nil
}

func (b *Builder) AddCompleted(tx Transfer) error {
	if tx.Status != "success" || tx.CreatedAt.After(b.report.AsOf) {
		return nil
	}
	at, _, code := completion(tx)
	if code != "" {
		b.issue(code, "global", nil)
		return nil
	}
	p, current := b.periodAt(*at)
	if p == nil {
		return nil
	}
	if !validMoney(tx) {
		date := at.In(timeutil.VietnamLocation).Format(time.DateOnly)
		b.issue("INVALID_MONEY", "completed", &date)
		return nil
	}
	if err := addMoney(&p.summary.Completed.TotalAmount, tx.Amount); err != nil {
		return err
	}
	if err := addMoney(&p.summary.Completed.TotalFee, tx.Fee); err != nil {
		return err
	}
	p.summary.Completed.Count++
	if current {
		hour := &b.report.Hourly.Completed[at.In(timeutil.VietnamLocation).Hour()]
		hour.Count++
		hour.TotalAmount += tx.Amount
		hour.TotalFee += tx.Fee
	}
	return nil
}

func (b *Builder) Finish(generatedAt time.Time) Report {
	for _, p := range []*period{&b.current, &b.previous} {
		if p.summary.Completed.Count > 0 {
			value := float64(p.summary.Completed.TotalAmount) / float64(p.summary.Completed.Count)
			p.summary.Completed.AverageAmount = &value
		}
		if p.summary.Risk.AssessedCount > 0 {
			value := p.score / float64(p.summary.Risk.AssessedCount)
			p.summary.Risk.AverageScore = &value
		}
		if p.summary.Created.TransactionCount > 0 {
			value := float64(p.summary.Risk.AssessedCount) / float64(p.summary.Created.TransactionCount)
			p.summary.Risk.Coverage = &value
		}
	}
	b.report.Summary, b.report.Previous = b.current.summary, b.previous.summary
	b.report.GeneratedAt = generatedAt.UTC()
	sort.Slice(b.report.Transactions, func(i, j int) bool {
		left, right := b.report.Transactions[i], b.report.Transactions[j]
		if left.CreatedAt.Equal(right.CreatedAt) {
			return left.TransactionID < right.TransactionID
		}
		return left.CreatedAt.After(right.CreatedAt)
	})
	b.report.TopRiskSources, b.report.TopRiskReceivers = topAccounts(b.sources), topAccounts(b.receivers)
	b.report.DataQuality = b.report.DataQuality[:0]
	for _, issue := range b.issues {
		b.report.DataQuality = append(b.report.DataQuality, issue)
	}
	sort.Slice(b.report.DataQuality, func(i, j int) bool {
		a, c := b.report.DataQuality[i], b.report.DataQuality[j]
		if a.Code != c.Code {
			return a.Code < c.Code
		}
		if a.Scope != c.Scope {
			return a.Scope < c.Scope
		}
		return dateValue(a.Date) < dateValue(c.Date)
	})
	return b.report
}

func topAccounts(values map[string]RiskAccount) []RiskAccount {
	items := make([]RiskAccount, 0, len(values))
	for _, item := range values {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		if items[i].Amount != items[j].Amount {
			return items[i].Amount > items[j].Amount
		}
		return items[i].AccountNo < items[j].AccountNo
	})
	if len(items) > 5 {
		items = items[:5]
	}
	return items
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func timePtr(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	utc := value.UTC()
	return &utc
}

func optionalTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	return timePtr(*value)
}

func dateValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
