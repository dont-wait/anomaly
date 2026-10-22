package queries

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type loanContractReaderStub struct {
	owner    string
	loanID   string
	limit    int
	afterID  string
	rows     []ContractListRow
	more     bool
	listErr  error
	getErr   error
	snapshot string
}

func (s *loanContractReaderStub) ListContractPage(_ context.Context, customerID string, limit int, _ time.Time, afterID string) ([]ContractListRow, bool, error) {
	s.owner, s.limit, s.afterID = customerID, limit, afterID
	return s.rows, s.more, s.listErr
}

func (s *loanContractReaderStub) ReadContract(_ context.Context, loanID, customerID string) (ContractDetail, error) {
	s.loanID, s.owner = loanID, customerID
	return ContractDetail{}, s.getErr
}

func TestLoanContractListCursorAndOwnership(t *testing.T) {
	id := uuid.NewString()
	created := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	reader := &loanContractReaderStub{more: true, rows: []ContractListRow{{
		Contract: ContractSummary{LoanID: id}, ApplicationID: id, ApplicationCreatedAt: created,
	}}}
	service := NewLoanContractQueryService(reader)

	page, err := service.List(context.Background(), "owner-1", 1, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].LoanID != id || page.NextCursor == nil || !page.EventualConsistency {
		t.Fatalf("unexpected first page: %+v, err=%v", page, err)
	}
	if reader.owner != "owner-1" || reader.limit != 1 {
		t.Fatal("customer owner and limit must reach the reader unchanged")
	}
	reader.more = false
	reader.rows = nil
	next, err := service.List(context.Background(), "owner-1", 1, *page.NextCursor)
	if err != nil || next.NextCursor != nil || next.Items == nil || reader.afterID != id {
		t.Fatalf("wrong cursor propagation: %+v, err=%v", next, err)
	}
	if _, err := service.List(context.Background(), "owner-2", 1, *page.NextCursor); !errors.Is(err, ErrInvalidLoanContractQuery) {
		t.Fatalf("cross-customer cursor accepted: %v", err)
	}
}

func TestLoanContractQueryRejectsInvalidRequests(t *testing.T) {
	service := NewLoanContractQueryService(&loanContractReaderStub{})
	for _, tt := range []struct {
		owner  string
		limit  int
		cursor string
	}{
		{"", 20, ""},
		{"owner", 0, ""},
		{"owner", 51, ""},
		{"owner", 20, "not-base64!"},
		{"owner", 20, string(make([]byte, 2049))},
	} {
		if _, err := service.List(context.Background(), tt.owner, tt.limit, tt.cursor); !errors.Is(err, ErrInvalidLoanContractQuery) {
			t.Fatalf("invalid list input not rejected: %+v, err=%v", tt, err)
		}
	}
	invalidID := base64.RawURLEncoding.EncodeToString(mustJSON(t, contractCursor{
		CreatedAt: time.Now(), ID: "not-uuid", Owner: "owner",
	}))
	if _, err := service.List(context.Background(), "owner", 20, invalidID); !errors.Is(err, ErrInvalidLoanContractQuery) {
		t.Fatal("invalid cursor ID accepted")
	}
	for _, id := range []string{"", "invalid", "ffffffffffffffffffffffff"} {
		if _, err := service.Get(context.Background(), id, "owner"); !errors.Is(err, ErrInvalidLoanContractQuery) {
			t.Fatalf("invalid contract ID %q accepted: %v", id, err)
		}
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestLoanContractQueryPropagatesReaderErrors(t *testing.T) {
	reader := &loanContractReaderStub{getErr: ErrLoanContractNotFound, listErr: ErrLoanContractNotReady}
	service := NewLoanContractQueryService(reader)
	_, err := service.List(context.Background(), "owner", 20, "")
	if !errors.Is(err, ErrLoanContractNotReady) {
		t.Fatalf("expected lag error, got %v", err)
	}
	id := uuid.NewString()
	_, err = service.Get(context.Background(), id, "owner")
	if !errors.Is(err, ErrLoanContractNotFound) || reader.loanID != id || reader.owner != "owner" {
		t.Fatalf("reader must authorize requested ID: id=%q owner=%q err=%v", reader.loanID, reader.owner, err)
	}
}

func (s *loanContractReaderStub) ContractSnapshot(
	_ context.Context,
	_ string,
) (string, error) {
	if s.snapshot != "" {
		return s.snapshot, nil
	}
	return "test-snapshot-v1", nil
}

func TestLoanContractListRejectsChangedSnapshot(t *testing.T) {
	id := uuid.NewString()
	reader := &loanContractReaderStub{
		snapshot: "revision-1",
		more:     true,
		rows: []ContractListRow{{
			ApplicationID:        id,
			ApplicationCreatedAt: time.Now().UTC(),
			Contract: ContractSummary{
				LoanID: id,
			},
		}},
	}

	service := NewLoanContractQueryService(reader)

	first, err := service.List(
		context.Background(), "owner", 1, "",
	)
	if err != nil || first.NextCursor == nil {
		t.Fatalf("first page: %+v, err=%v", first, err)
	}

	// A previously non-disbursed application enters the list.
	reader.snapshot = "revision-2"

	_, err = service.List(
		context.Background(), "owner", 1, *first.NextCursor,
	)
	if !errors.Is(err, ErrLoanContractPageChanged) {
		t.Fatalf("expected snapshot conflict, got %v", err)
	}
}
