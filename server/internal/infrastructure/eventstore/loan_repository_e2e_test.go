//go:build e2e

package eventstore_test

import (
	"context"
	"strings"
	"testing"
	"time"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	"github.com/google/uuid"
)

func TestLoanRepositoryE2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Minute,
	)
	defer cancel()

	// Tận dụng Testcontainers có sẵn trong bank_e2e_test.go.
	esClient := startEventStore(t, ctx)

	repo := eventstore.NewLoanRepository(esClient)

	applicationID := uuid.NewString()

	// 1. Kiểm tra hồ sơ chưa tồn tại.
	notFound, err := repo.FindByID(ctx, applicationID)
	if err != nil {
		t.Fatalf("find missing application: %v", err)
	}

	if notFound != nil {
		t.Fatal("expected nil for missing application")
	}

	// 2. Tạo hồ sơ vay hợp lệ bằng Domain.
	draft, err := loandomain.NewDraftApplication(
		applicationID,
		"LA-"+applicationID,
		"customer-001",
		"package-001",
		"account-001",
		10_000_000,
		12,
	)
	if err != nil {
		t.Fatalf("create domain draft: %v", err)
	}

	// 3. Ghi LoanApplicationCreated vào EventStoreDB.
	if err := repo.Create(ctx, draft); err != nil {
		t.Fatalf("persist loan application: %v", err)
	}

	// 4. Đọc stream và khôi phục hồ sơ.
	loaded, revision, err := repo.FindByIDWithRevision(
		ctx,
		applicationID,
	)
	if err != nil {
		t.Fatalf("replay created application: %v", err)
	}

	if loaded == nil {
		t.Fatal("application was not restored")
	}

	if revision != 0 {
		t.Fatalf("expected revision 0, got %d", revision)
	}

	if loaded.Id != draft.Id {
		t.Fatalf("incorrect application ID: %s", loaded.Id)
	}

	if loaded.RequestedAmount != 10_000_000 {
		t.Fatalf(
			"incorrect requested amount: %d",
			loaded.RequestedAmount,
		)
	}

	if loaded.TermMonths != 12 {
		t.Fatalf("incorrect term: %d", loaded.TermMonths)
	}

	if loaded.Status != loandomain.ApplicationStatusDraft {
		t.Fatalf("incorrect status: %s", loaded.Status)
	}

	if !loaded.CreatedAt.Equal(draft.CreatedAt) {
		t.Fatal("creation timestamp was not restored")
	}

	// 5. Không cho phép tạo trùng stream.
	if err := repo.Create(ctx, draft); err == nil {
		t.Fatal("expected duplicate creation to fail")
	}

	// 6. Chuẩn bị chứng từ hợp lệ.
	uploadedAt := time.Now().UTC()

	document := loandomain.ApplicationDocument{
		DocumentId:         "doc-" + uuid.NewString(),
		Type:               "IDENTITY_FRONT",
		StorageKey:         "loans/" + applicationID + "/identity-front",
		MIMEType:           "image/jpeg",
		SizeBytes:          2048,
		SHA256:             strings.Repeat("a", 64),
		VerificationStatus: "pending",
		UploadedAt:         uploadedAt,
	}

	// Dùng Domain để kiểm tra chứng từ trước khi tạo event.
	preview := *loaded

	if err := preview.AddDocument(document); err != nil {
		t.Fatalf("domain rejected document: %v", err)
	}

	// Payload tương ứng với LoanDocumentAdded.
	// Dùng struct vì payload trong infrastructure
	// không export ra ngoài package eventstore.
	payload := struct {
		Document  loandomain.ApplicationDocument `json:"document"`
		UpdatedAt time.Time                      `json:"updatedAt"`
	}{
		Document:  document,
		UpdatedAt: preview.UpdatedAt,
	}

	// 7. Ghi LoanDocumentAdded với revision hiện tại.
	if err := repo.Append(
		ctx,
		applicationID,
		revision,
		eventstore.EventLoanDocumentAdded,
		payload,
	); err != nil {
		t.Fatalf("append document event: %v", err)
	}

	// 8. Đọc lại và kiểm tra revision tăng.
	updated, newRevision, err := repo.FindByIDWithRevision(
		ctx,
		applicationID,
	)
	if err != nil {
		t.Fatalf("replay updated application: %v", err)
	}

	if updated == nil {
		t.Fatal("updated application was not restored")
	}

	if newRevision != 1 {
		t.Fatalf(
			"expected revision 1, got %d",
			newRevision,
		)
	}

	if len(updated.Documents) != 1 {
		t.Fatalf(
			"expected 1 document, got %d",
			len(updated.Documents),
		)
	}

	if updated.Documents[0].DocumentId != document.DocumentId {
		t.Fatal("document ID was not restored correctly")
	}

	if !updated.UpdatedAt.Equal(preview.UpdatedAt) {
		t.Fatal("document update timestamp was not restored")
	}

	// 9. Ghi bằng revision cũ phải thất bại.
	err = repo.Append(
		ctx,
		applicationID,
		0,
		eventstore.EventLoanDocumentAdded,
		payload,
	)

	if err == nil {
		t.Fatal("expected stale revision to be rejected")
	}

	// 10. Không cho phép phê duyệt hồ sơ còn draft.
	decisionPayload := map[string]any{
		"decidedAt": time.Now().UTC(),
		"updatedAt": time.Now().UTC(),
	}

	err = repo.Append(
		ctx,
		applicationID,
		newRevision,
		eventstore.EventLoanApplicationApproved,
		decisionPayload,
	)

	if err == nil {
		t.Fatal("expected invalid approval event to fail")
	}

	// 11. Các thao tác thất bại không được thay đổi stream.
	finalApp, finalRevision, err := repo.FindByIDWithRevision(
		ctx,
		applicationID,
	)
	if err != nil {
		t.Fatalf("final replay: %v", err)
	}

	if finalApp == nil {
		t.Fatal("final application is nil")
	}

	if finalRevision != 1 {
		t.Fatalf(
			"expected final revision 1, got %d",
			finalRevision,
		)
	}

	if finalApp.Status != loandomain.ApplicationStatusDraft {
		t.Fatal("application status changed unexpectedly")
	}

	if len(finalApp.Documents) != 1 {
		t.Fatal("documents changed unexpectedly")
	}

	t.Log("Loan Repository E2E passed")
}
