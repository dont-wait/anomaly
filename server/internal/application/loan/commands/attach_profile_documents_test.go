package commands

import (
	"context"
	"errors"
	"strings"
	"testing"

	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
)

type profileDocumentProvider struct {
	documents []loandomain.ApplicationDocument
}

func (p profileDocumentProvider) VerifiedProfileDocuments(context.Context, string) ([]loandomain.ApplicationDocument, error) {
	return p.documents, nil
}

type attachRepo struct {
	app       *loandomain.Application
	revisions []uint64
}

func (r *attachRepo) FindByIDWithRevision(context.Context, string) (*loandomain.Application, uint64, error) {
	return r.app, 5, nil
}

func (r *attachRepo) Append(_ context.Context, _ string, revision uint64, _ string, _ any) error {
	r.revisions = append(r.revisions, revision)
	return nil
}

type snapshotStorage struct {
	copied   map[string]string
	failFor  string
	verified int
}

func (s *snapshotStorage) CopyLoanDocument(_ context.Context, source, target, _ string) error {
	if source == s.failFor {
		return errors.New("storage down")
	}
	if s.copied == nil {
		s.copied = map[string]string{}
	}
	s.copied[source] = target
	return nil
}

func (s *snapshotStorage) VerifyLoanDocument(context.Context, string, string, loandomain.ApplicationDocument) error {
	s.verified++
	return nil
}

func identityDocument(documentType string) loandomain.ApplicationDocument {
	return loandomain.ApplicationDocument{Type: documentType, StorageKey: "kyc/" + documentType + ".jpg", MIMEType: "image/jpeg", SizeBytes: 10, SHA256: "abc"}
}

func TestAttachProfileDocumentsAddsMissingAsPending(t *testing.T) {
	repo := &attachRepo{app: &loandomain.Application{
		Id: "app", CustomerId: "me", Status: loandomain.ApplicationStatusDraft,
		Documents: []loandomain.ApplicationDocument{{DocumentId: "old", Type: "IDENTITY_BACK", VerificationStatus: "rejected"}},
	}}
	storage := &snapshotStorage{}
	handler := &AttachProfileDocumentsCommandHandler{Repo: repo, Storage: storage, Provider: profileDocumentProvider{[]loandomain.ApplicationDocument{
		identityDocument("IDENTITY_FRONT"), identityDocument("IDENTITY_BACK"), {Type: "IDENTITY_NO_METADATA"},
	}}}

	result, err := handler.Handle(context.Background(), "app", "me")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Documents) != 2 || result.Documents[1].Type != "IDENTITY_FRONT" || result.Documents[1].VerificationStatus != "pending" {
		t.Fatalf("documents = %#v", result.Documents)
	}
	if len(repo.revisions) != 1 || repo.revisions[0] != 5 {
		t.Fatalf("revisions = %v", repo.revisions)
	}
	key := result.Documents[1].StorageKey
	if !strings.HasPrefix(key, "loans/me/app/") || !strings.HasSuffix(key, ".jpg") || storage.copied["kyc/IDENTITY_FRONT.jpg"] != key || storage.verified != 1 {
		t.Fatalf("snapshot key = %q, copied = %v, verified = %d", key, storage.copied, storage.verified)
	}
}

func TestAttachProfileDocumentsContinuesAfterOneCopyFails(t *testing.T) {
	repo := &attachRepo{app: &loandomain.Application{Id: "app", CustomerId: "me", Status: loandomain.ApplicationStatusDraft}}
	handler := &AttachProfileDocumentsCommandHandler{Repo: repo, Storage: &snapshotStorage{failFor: "kyc/IDENTITY_FRONT.jpg"}, Provider: profileDocumentProvider{[]loandomain.ApplicationDocument{
		identityDocument("IDENTITY_FRONT"), identityDocument("IDENTITY_BACK"),
	}}}
	result, err := handler.Handle(context.Background(), "app", "me")
	if err != nil || len(result.Documents) != 1 || result.Documents[0].Type != "IDENTITY_BACK" {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
}

func TestAttachProfileDocumentsRespectsOwnerAndStatus(t *testing.T) {
	provider := profileDocumentProvider{[]loandomain.ApplicationDocument{identityDocument("IDENTITY_FRONT")}}
	repo := &attachRepo{app: &loandomain.Application{Id: "app", CustomerId: "owner", Status: loandomain.ApplicationStatusDraft}}
	handler := &AttachProfileDocumentsCommandHandler{Repo: repo, Storage: &snapshotStorage{}, Provider: provider}
	if _, err := handler.Handle(context.Background(), "app", "other"); err == nil {
		t.Fatal("expected an error for another customer")
	}
	repo.app.Status = loandomain.ApplicationStatusSubmitted
	if _, err := handler.Handle(context.Background(), "app", "owner"); err != nil || len(repo.revisions) != 0 {
		t.Fatalf("err = %v, revisions = %v", err, repo.revisions)
	}
}
