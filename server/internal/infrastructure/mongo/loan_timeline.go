package mongo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
	"github.com/dont-wait/anomaly/internal/domain/timeline"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const loanTimelineCollection = "loan_account_timeline"

var loanEventSummaries = map[string]string{
	"LoanApplicationCreated":   "Hồ sơ vay được tạo",
	"LoanDocumentAdded":        "Hồ sơ vay được bổ sung chứng từ",
	"LoanDocumentReviewed":     "Chứng từ vay được duyệt",
	"LoanApplicationDeleted":   "Hồ sơ vay chưa gửi bị xóa",
	"LoanApplicationSubmitted": "Hồ sơ vay được gửi",
	"LoanReviewStarted":        "Bắt đầu thẩm định khoản vay",
	"LoanApplicationAssessed":  "Khoản vay được thẩm định",
	"LoanApplicationApproved":  "Hồ sơ vay được phê duyệt",
	"LoanApplicationRejected":  "Hồ sơ vay bị từ chối",
	"LoanOfferIssued":          "Đề nghị khoản vay được phát hành",
	"LoanOfferAccepted":        "Đề nghị khoản vay được chấp nhận",
	"LoanOfferRejected":        "Đề nghị khoản vay bị từ chối",
	"LoanContractConfirmed":    "Hợp đồng vay được xác nhận",
	"LoanDisbursementPrepared": "Khoản vay được chuẩn bị giải ngân",
	"LoanDisbursed":            "Khoản vay được giải ngân",
}

func (m *LoanReadModel) EnsureLoanTimelineIndexes(ctx context.Context) error {
	_, err := m.db.Collection(loanTimelineCollection).Indexes().CreateMany(ctx, []mongodrv.IndexModel{
		{
			Keys: bson.D{
				{Key: "account_id", Value: 1},
				{Key: "occurred_at", Value: -1},
				{Key: "_id", Value: -1},
			},
		},
	})
	return err
}

// SaveLoanTimelineEvent stores one masked loan event idempotently. The caller
// only invokes it after the loan aggregate replay has succeeded.
func (m *LoanReadModel) SaveLoanTimelineEvent(
	ctx context.Context,
	app *loandomain.Application,
	eventID, eventType string,
	revision uint64,
	occurredAt time.Time,
	payload []byte,
) error {
	if app == nil || strings.TrimSpace(app.Id) == "" || strings.TrimSpace(app.DisbursementAccountId) == "" {
		return fmt.Errorf("loan timeline requires an application and disbursement account")
	}
	if strings.TrimSpace(eventID) == "" || strings.TrimSpace(eventType) == "" {
		return fmt.Errorf("loan timeline event metadata is required")
	}
	if occurredAt.IsZero() {
		occurredAt = app.UpdatedAt
	}

	details := map[string]any{
		"loanId":          app.Id,
		"applicationNo":   app.ApplicationNo,
		"requestedAmount": app.RequestedAmount,
		"termMonths":      app.TermMonths,
	}
	addSafeLoanDetails(details, eventType, payload)

	event := timeline.LoanEvent{
		EventID:        eventID,
		EventType:      eventType,
		LoanID:         app.Id,
		AccountID:      app.DisbursementAccountId,
		SourceStream:   "loan-application-" + app.Id,
		SourceRevision: revision,
		OccurredAt:     occurredAt.UTC(),
		Summary:        loanEventSummaries[eventType],
		Details:        details,
	}
	if event.Summary == "" {
		return fmt.Errorf("unsupported loan timeline event %q", eventType)
	}

	_, err := m.db.Collection(loanTimelineCollection).ReplaceOne(
		ctx,
		bson.M{"_id": event.EventID},
		event,
		options.Replace().SetUpsert(true),
	)
	return err
}

func addSafeLoanDetails(details map[string]any, eventType string, payload []byte) {
	var raw map[string]any
	if json.Unmarshal(payload, &raw) != nil {
		return
	}
	for _, key := range []string{"decision", "totalScore", "policyVersion", "annualRateBps", "method"} {
		if value, ok := raw[key]; ok {
			details[key] = value
		}
	}
	// Assessment payloads contain the decision fields at the top level. The
	// document-added payload is intentionally never copied into the timeline.
	_ = eventType
}

func (m *LoanReadModel) NextLoanTimelineEvent(
	ctx context.Context,
	accountID string,
	beforeAt time.Time,
	beforeEventID string,
) (*timeline.LoanEvent, error) {
	filter := bson.M{"account_id": accountID}
	if !beforeAt.IsZero() {
		filter["$or"] = bson.A{
			bson.M{"occurred_at": bson.M{"$lt": beforeAt}},
			bson.M{"occurred_at": beforeAt, "_id": bson.M{"$lt": beforeEventID}},
		}
	}

	var event timeline.LoanEvent
	err := m.db.Collection(loanTimelineCollection).FindOne(
		ctx,
		filter,
		options.FindOne().SetSort(bson.D{
			{Key: "occurred_at", Value: -1},
			{Key: "_id", Value: -1},
		}),
	).Decode(&event)
	if err == mongodrv.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &event, nil
}
