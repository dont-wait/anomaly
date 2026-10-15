//go:build e2e

package eventstore_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	"github.com/google/uuid"
)

// Chuẩn bị một hồ sơ đã đủ 7 chứng từ,
// đã submit và đang trong trạng thái under_review.
func createLoanUnderReviewForBatchTest(
	t *testing.T,
	ctx context.Context,
	repo *eventstore.LoanRepository,
) (string, uint64, time.Time) {
	t.Helper()

	id := uuid.NewString()

	draft, err := loandomain.NewDraftApplication(
		id,
		"LA-"+id,
		"customer-001",
		"package-001",
		"account-001",
		10_000_000,
		12,
	)
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}

	if err := repo.Create(ctx, draft); err != nil {
		t.Fatalf("save draft: %v", err)
	}

	baseTime := draft.CreatedAt

	requiredTypes := []string{
		"IDENTITY_FRONT",
		"IDENTITY_BACK",
		"VNEID_PERSONAL",
		"VNEID_RESIDENCE",
		"VSSID_PARTICIPATION",
		"VSSID_DETAIL",
		"SALARY_STATEMENT_3_MONTHS",
	}

	events := make(
		[]eventstore.LoanAppendEvent,
		0,
		len(requiredTypes)+2,
	)

	for i, documentType := range requiredTypes {
		at := baseTime.Add(
			time.Duration(i+1) * time.Second,
		)

		document := loandomain.ApplicationDocument{
			DocumentId:         fmt.Sprintf("doc-%d", i+1),
			Type:               documentType,
			StorageKey:         fmt.Sprintf("loans/%s/doc-%d", id, i+1),
			MIMEType:           "application/pdf",
			SizeBytes:          1024,
			SHA256:             strings.Repeat("a", 64),
			VerificationStatus: "pending",
			UploadedAt:         at,
		}

		events = append(
			events,
			eventstore.LoanAppendEvent{
				EventType: eventstore.EventLoanDocumentAdded,
				Payload: map[string]any{
					"document":  document,
					"updatedAt": at,
				},
			},
		)
	}

	submittedAt := baseTime.Add(10 * time.Second)

	events = append(
		events,
		eventstore.LoanAppendEvent{
			EventType: eventstore.EventLoanApplicationSubmitted,
			Payload: map[string]any{
				"submittedAt": submittedAt,
				"updatedAt":   submittedAt,
			},
		},
	)

	reviewAt := baseTime.Add(11 * time.Second)

	events = append(
		events,
		eventstore.LoanAppendEvent{
			EventType: eventstore.EventLoanReviewStarted,
			Payload: map[string]any{
				"reviewStartedAt": reviewAt,
				"updatedAt":       reviewAt,
			},
		},
	)

	// Revision ban đầu là 0.
	// Ghi 7 DocumentAdded + Submitted + ReviewStarted.
	if err := repo.AppendBatch(
		ctx,
		id,
		0,
		events...,
	); err != nil {
		t.Fatalf("prepare under_review application: %v", err)
	}

	app, revision, err := repo.FindByIDWithRevision(ctx, id)
	if err != nil {
		t.Fatalf("read prepared application: %v", err)
	}

	if app == nil {
		t.Fatal("prepared application not found")
	}

	if app.Status != loandomain.ApplicationStatusUnderReview {
		t.Fatalf(
			"expected under_review, got %s",
			app.Status,
		)
	}

	if len(app.Documents) != 7 {
		t.Fatalf(
			"expected 7 documents, got %d",
			len(app.Documents),
		)
	}

	if revision != uint64(len(events)) {
		t.Fatalf(
			"expected revision %d, got %d",
			len(events),
			revision,
		)
	}

	return id, revision, baseTime
}

// Tạo cặp sự kiện phê duyệt tự động hợp lệ.
func automaticLoanApprovalBatch(
	baseTime time.Time,
) []eventstore.LoanAppendEvent {
	assessedAt := baseTime.Add(12 * time.Second)
	approvedAt := baseTime.Add(13 * time.Second)

	return []eventstore.LoanAppendEvent{
		{
			EventType: eventstore.EventLoanApplicationAssessed,
			Payload: map[string]any{
				"dtiScore":       30,
				"insuranceScore": 25,
				"historyScore":   35,
				"totalScore":     90,
				"decision":       loandomain.AssessmentAutoEligible,
				"reasons":        []string{},
				"policyVersion":  "loan-policy-v1",
				"assessedAt":     assessedAt,
				"updatedAt":      assessedAt,
			},
		},
		{
			EventType: eventstore.EventLoanApplicationApproved,
			Payload: map[string]any{
				"decidedAt": approvedAt,
				"updatedAt": approvedAt,
			},
		},
	}
}

// Kiểm tra AppendBatch với EventStoreDB thật.
func TestLoanRepositoryAppendBatchE2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Minute,
	)
	defer cancel()

	esClient := startEventStore(t, ctx)
	repo := eventstore.NewLoanRepository(esClient)

	// Trường hợp 1: Hai sự kiện hợp lệ được lưu cùng nhau.
	t.Run("valid batch commits both events", func(t *testing.T) {
		id, revision, baseTime := createLoanUnderReviewForBatchTest(t, ctx, repo)

		events := automaticLoanApprovalBatch(baseTime)

		err := repo.AppendBatch(
			ctx,
			id,
			revision,
			events...,
		)
		if err != nil {
			t.Fatalf("append valid batch: %v", err)
		}

		app, newRevision, err := repo.FindByIDWithRevision(
			ctx,
			id,
		)
		if err != nil {
			t.Fatalf("read approved application: %v", err)
		}

		if app == nil {
			t.Fatal("approved application not found")
		}

		if newRevision != revision+2 {
			t.Fatalf(
				"expected revision %d, got %d",
				revision+2,
				newRevision,
			)
		}

		if app.Status != loandomain.ApplicationStatusApproved {
			t.Fatalf(
				"expected approved, got %s",
				app.Status,
			)
		}

		if app.AssessmentScore != 90 {
			t.Fatalf(
				"expected score 90, got %d",
				app.AssessmentScore,
			)
		}

		if app.AssessmentDecision !=
			loandomain.AssessmentAutoEligible {
			t.Fatal("incorrect assessment decision")
		}

		if app.AssessedAt == nil || app.ApprovedAt == nil {
			t.Fatal("assessment or approval timestamp missing")
		}
	})

	// Trường hợp 2: Event thứ hai không hợp lệ.
	// Không được ghi cả Event thứ nhất.
	t.Run("invalid second event writes nothing", func(t *testing.T) {
		id, revision, baseTime := createLoanUnderReviewForBatchTest(t, ctx, repo)

		events := automaticLoanApprovalBatch(baseTime)

		// Kết quả chấm điểm là auto_approve_eligible,
		// nhưng event thứ hai lại yêu cầu từ chối.
		events[1].EventType = eventstore.EventLoanApplicationRejected

		err := repo.AppendBatch(
			ctx,
			id,
			revision,
			events...,
		)
		if err == nil {
			t.Fatal("expected invalid batch to fail")
		}

		app, finalRevision, err := repo.FindByIDWithRevision(
			ctx,
			id,
		)
		if err != nil {
			t.Fatalf("read after failed batch: %v", err)
		}

		if app == nil {
			t.Fatal("application not found")
		}

		if finalRevision != revision {
			t.Fatalf(
				"stream changed: revision %d -> %d",
				revision,
				finalRevision,
			)
		}

		if app.AssessedAt != nil {
			t.Fatal("assessment was partially committed")
		}

		if app.Status != loandomain.ApplicationStatusUnderReview {
			t.Fatal("failed batch changed application status")
		}
	})

	// Trường hợp 3: Revision không còn hợp lệ.
	t.Run("stale revision writes nothing", func(t *testing.T) {
		id, revision, baseTime := createLoanUnderReviewForBatchTest(t, ctx, repo)

		events := automaticLoanApprovalBatch(baseTime)

		err := repo.AppendBatch(
			ctx,
			id,
			revision-1,
			events...,
		)
		if err == nil {
			t.Fatal("expected stale revision conflict")
		}

		app, finalRevision, err := repo.FindByIDWithRevision(
			ctx,
			id,
		)
		if err != nil {
			t.Fatalf("read after revision conflict: %v", err)
		}

		if app == nil {
			t.Fatal("application not found")
		}

		if finalRevision != revision {
			t.Fatalf(
				"expected revision %d, got %d",
				revision,
				finalRevision,
			)
		}

		if app.AssessedAt != nil {
			t.Fatal("stale batch unexpectedly committed")
		}

		if app.Status != loandomain.ApplicationStatusUnderReview {
			t.Fatal("revision conflict changed application status")
		}
	})
}
