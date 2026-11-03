package catalog

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/dont-wait/anomaly/internal/domain/catalog"
)

type fakeStore struct {
	Store
	versions    []domain.Package
	inserted    *domain.Package
	replaced    *domain.Package
	transitions []Transition
}

func (f *fakeStore) Versions(context.Context, domain.Kind, string) ([]domain.Package, error) {
	return f.versions, nil
}

func (f *fakeStore) InsertDraft(_ context.Context, p domain.Package) error {
	f.inserted = &p
	return nil
}

func (f *fakeStore) ReplaceDraft(_ context.Context, p domain.Package) error {
	f.replaced = &p
	return nil
}

func (f *fakeStore) Publish(_ context.Context, t Transition) (domain.Package, error) {
	f.transitions = append(f.transitions, t)
	return domain.Package{}, nil
}

func (f *fakeStore) Retire(_ context.Context, t Transition) (domain.Package, error) {
	f.transitions = append(f.transitions, t)
	return domain.Package{}, nil
}

func activeLoan() domain.Package {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return domain.Package{
		Kind: domain.KindLoan, Code: "P", Version: 2, Status: domain.StatusActive, Name: "Vay",
		MinAmount: 1, MaxAmount: 2, Terms: []domain.TermOption{{Months: 12}},
		PublishedBy: "a", PublishedAt: &at,
	}
}

func TestCreateDraftCopiesLatestAndClearsPublishing(t *testing.T) {
	store := &fakeStore{versions: []domain.Package{activeLoan()}}
	draft, err := (&Service{Store: store}).CreateDraft(context.Background(), domain.KindLoan, "P", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if draft.Version != 3 || draft.Status != domain.StatusDraft || draft.PublishedAt != nil || draft.CreatedBy != "admin" || store.inserted == nil {
		t.Fatalf("draft = %+v", draft)
	}

	store.versions = []domain.Package{draft}
	if _, err := (&Service{Store: store}).CreateDraft(context.Background(), domain.KindLoan, "P", "admin"); !errors.Is(err, ErrDraftExists) {
		t.Fatalf("err = %v", err)
	}
	store.versions = nil
	if _, err := (&Service{Store: store}).CreateDraft(context.Background(), domain.KindLoan, "P", "admin"); !errors.Is(err, ErrPackageNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdateDraftValidatesAndOnlyTouchesDrafts(t *testing.T) {
	store := &fakeStore{versions: []domain.Package{activeLoan()}}
	service := &Service{Store: store}
	input := DraftInput{Name: "Mới", MinAmount: 5, MaxAmount: 10, Terms: []domain.TermOption{{Months: 24}}}
	if _, err := service.UpdateDraft(context.Background(), domain.KindLoan, "P", input); !errors.Is(err, ErrNoDraft) {
		t.Fatalf("err = %v", err)
	}
	draft := activeLoan()
	draft.Status = domain.StatusDraft
	store.versions = []domain.Package{draft}
	input.MaxAmount = 1
	if _, err := service.UpdateDraft(context.Background(), domain.KindLoan, "P", input); !errors.Is(err, domain.ErrInvalidPackage) {
		t.Fatalf("err = %v", err)
	}
	input.MaxAmount = 10
	if _, err := service.UpdateDraft(context.Background(), domain.KindLoan, "P", input); err != nil || store.replaced == nil || store.replaced.MaxAmount != 10 {
		t.Fatalf("replaced = %+v, err = %v", store.replaced, err)
	}
}

func TestPublishValidatesDraftAndRequiresKey(t *testing.T) {
	bad := activeLoan()
	bad.Status, bad.MaxAmount = domain.StatusDraft, 0
	store := &fakeStore{versions: []domain.Package{bad}}
	service := &Service{Store: store}
	if _, err := service.Publish(context.Background(), domain.KindLoan, "P", "a", "", 2); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("err = %v", err)
	}
	if _, err := service.Publish(context.Background(), domain.KindLoan, "P", "a", "k", 2); !errors.Is(err, domain.ErrInvalidPackage) {
		t.Fatalf("err = %v", err)
	}
	if len(store.transitions) != 0 {
		t.Fatal("invalid draft must not reach the store")
	}
}

func TestRetireRequiresReason(t *testing.T) {
	store := &fakeStore{}
	service := &Service{Store: store}
	if _, err := service.Retire(context.Background(), domain.KindLoan, "P", "a", "k", " ", 2); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("err = %v", err)
	}
	if _, err := service.Retire(context.Background(), domain.KindLoan, "P", "a", "k", "Hết hạn mức", 2); err != nil || len(store.transitions) != 1 {
		t.Fatalf("err = %v", err)
	}
}

func TestSummarizeReportsEffectiveState(t *testing.T) {
	active := activeLoan()
	draft := active
	draft.Version, draft.Status = 3, domain.StatusDraft
	got := summarize(domain.KindLoan, "P", []domain.Package{draft, active})
	if got.Status != domain.StatusActive || got.ActiveVersion != 2 || got.DraftVersion != 3 {
		t.Fatalf("summary = %+v", got)
	}
	retired := active
	retired.Status = domain.StatusRetired
	got = summarize(domain.KindLoan, "P", []domain.Package{retired})
	if got.Status != domain.StatusRetired || got.ActiveVersion != 0 {
		t.Fatalf("summary = %+v", got)
	}
	got = summarize(domain.KindLoan, "P", []domain.Package{draft})
	if got.Status != domain.StatusDraft {
		t.Fatalf("summary = %+v", got)
	}
}

func TestSimulations(t *testing.T) {
	service := &Service{Now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }}
	loan, err := service.SimulateLoan(10_000_000, 20_000_000, "KV03", 12)
	if err != nil || loan.AnnualRateBps != 1500 || len(loan.Schedule) != 12 || loan.TotalPayment <= 10_000_000 {
		t.Fatalf("loan = %+v, err = %v", loan, err)
	}
	if _, err := service.SimulateLoan(10_000_000, 20_000_000, "XX", 12); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("err = %v", err)
	}
	saved, err := service.SimulateSavings(100_000_000, 520, 12)
	if err != nil || saved.Interest != 5_200_000 || saved.Total != 105_200_000 {
		t.Fatalf("savings = %+v, err = %v", saved, err)
	}
}

type deleteStore struct {
	fakeStore
	usage   map[string]int64
	deleted string
	dupe    bool
}

func (f *deleteStore) Usage(context.Context, domain.Kind, string) (map[string]int64, error) {
	return f.usage, nil
}

func (f *deleteStore) DeletePackage(_ context.Context, _ domain.Kind, code, _ string, _ time.Time) error {
	f.deleted = code
	return nil
}

func (f *deleteStore) InsertDraft(ctx context.Context, p domain.Package) error {
	if f.dupe {
		return ErrDraftExists
	}
	return f.fakeStore.InsertDraft(ctx, p)
}

func TestCreatePackageStartsAsDraftAndRejectsBadInput(t *testing.T) {
	store := &deleteStore{}
	service := &Service{Store: store}
	in := CreateInput{Code: " vay_mua_xe ", DraftInput: DraftInput{Name: "Vay mua xe", MinAmount: 1, MaxAmount: 2, Terms: []domain.TermOption{{Months: 12}}}}
	created, err := service.CreatePackage(context.Background(), domain.KindLoan, in, "admin")
	if err != nil || created.Code != "VAY_MUA_XE" || created.Version != 1 || created.Status != domain.StatusDraft || created.CreatedBy != "admin" {
		t.Fatalf("created = %+v, err = %v", created, err)
	}
	bad := in
	bad.Code = "x y"
	if _, err := service.CreatePackage(context.Background(), domain.KindLoan, bad, "admin"); !errors.Is(err, domain.ErrInvalidPackage) {
		t.Fatalf("bad code err = %v", err)
	}
	bad = in
	bad.Terms = nil
	if _, err := service.CreatePackage(context.Background(), domain.KindLoan, bad, "admin"); !errors.Is(err, domain.ErrInvalidPackage) {
		t.Fatalf("no terms err = %v", err)
	}
	store.dupe = true
	if _, err := service.CreatePackage(context.Background(), domain.KindLoan, in, "admin"); !errors.Is(err, ErrCodeExists) {
		t.Fatalf("duplicate err = %v", err)
	}
}

func TestDeletePackageOnlyWhenNotOnSaleAndUnused(t *testing.T) {
	store := &deleteStore{}
	service := &Service{Store: store}
	ctx := context.Background()
	if err := service.DeletePackage(ctx, domain.KindLoan, "P", "admin"); !errors.Is(err, ErrPackageNotFound) {
		t.Fatalf("missing err = %v", err)
	}
	store.versions = []domain.Package{activeLoan()}
	if err := service.DeletePackage(ctx, domain.KindLoan, "P", "admin"); !errors.Is(err, ErrPackageActive) {
		t.Fatalf("active err = %v", err)
	}
	retired := activeLoan()
	retired.Status = domain.StatusRetired
	store.versions = []domain.Package{retired}
	store.usage = map[string]int64{"rejected": 1}
	if err := service.DeletePackage(ctx, domain.KindLoan, "P", "admin"); !errors.Is(err, ErrPackageInUse) || store.deleted != "" {
		t.Fatalf("used err = %v, deleted = %q", err, store.deleted)
	}
	store.usage = nil
	if err := service.DeletePackage(ctx, domain.KindLoan, "P", "admin"); err != nil || store.deleted != "P" {
		t.Fatalf("err = %v, deleted = %q", err, store.deleted)
	}
}
