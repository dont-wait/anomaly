package queries

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/domain/savings"
)

type testSavingsStore struct {
	revision int64
	rows     []SavingsContract
	notices  []SavingsNotice
	reads    int
}

func (s *testSavingsStore) Snapshot(context.Context) (int64, error) { return s.revision, nil }
func (s *testSavingsStore) List(_ context.Context, customer, status string, limit int, after *SavingsCursor, now time.Time) ([]SavingsContract, error) {
	s.reads++
	var rows []SavingsContract
	for _, c := range s.rows {
		if c.CustomerID != customer {
			continue
		}
		if after != nil && !c.OpenedAt.Before(after.OpenedAt) && (!c.OpenedAt.Equal(after.OpenedAt) || c.SavingsID >= after.ID) {
			continue
		}
		switch status {
		case "settled":
			if c.Status != savings.ContractSettled {
				continue
			}
		case "active":
			if c.Status != savings.ContractActive || !c.MaturityAt.After(now) {
				continue
			}
		case "matured":
			if c.Status != savings.ContractActive || c.MaturityAt.After(now) {
				continue
			}
		}
		rows = append(rows, c)
		if len(rows) >= limit {
			break
		}
	}
	return rows, nil
}

func (s *testSavingsStore) Detail(_ context.Context, _ string, id string) (*SavingsContract, error) {
	s.reads++
	// Deliberately do NOT filter by customer: test the service ownership guard
	// independently of the MongoDB repository's additional protection.
	for _, c := range s.rows {
		if c.SavingsID == id {
			return &c, nil
		}
	}
	return nil, ErrContractNotFound
}

func (s *testSavingsStore) History(_ context.Context, _, _ string) ([]SavingsHistoryItem, error) {
	return nil, nil
}

func (s *testSavingsStore) Notifications(_ context.Context, _ string, _ int) ([]SavingsNotice, error) {
	return s.notices, nil
}

type testReadCache struct{ bytes map[string][]byte }

func (c *testReadCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	b, ok := c.bytes[key]
	return b, ok, nil
}

func (c *testReadCache) Set(_ context.Context, key string, data []byte, _ time.Duration) error {
	c.bytes[key] = data
	return nil
}

func testContract(id, customer string, status savings.ContractStatus, when time.Time) SavingsContract {
	c := SavingsContract{SavingsID: id, SavingsNo: "SV" + id, CustomerID: customer, PackageID: "savings_standard", SourceAccountID: "source", Principal: 10_000_000, TermMonths: 12, AnnualRateBps: 520, OpenedAt: when.AddDate(-1, 0, 0), MaturityAt: when, Status: status, FinancialRevision: 3}
	if status == savings.ContractSettled {
		c.Settlement = &SavingsSettlement{TransactionID: "tx", InterestAmount: 520_000, TotalAmount: 10_520_000, SettledAt: when}
	}
	return c
}

func TestSavingsReadOwnershipAndMaturityWithoutScheduler(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2027, 10, 24, 10, 0, 0, 0, time.UTC)
	store := &testSavingsStore{revision: 10, rows: []SavingsContract{
		testContract("own", "cust-1", savings.ContractActive, now.Add(-time.Second)),
		testContract("foreign", "cust-2", savings.ContractActive, now.Add(-time.Hour)),
	}}
	service := &SavingsReadService{Accounts: fakeAccounts{accounts: map[string]*account.UserAccount{"actor": activePayment("actor", "cust-1", 0)}}, Store: store, Now: func() time.Time { return now }}
	c, err := service.Detail(ctx, "actor", "own")
	if err != nil || c.Status != savings.ContractMatured {
		t.Fatalf("not matured at read: %+v %v", c, err)
	}
	if _, err := service.Detail(ctx, "actor", "foreign"); !errors.Is(err, ErrContractNotFound) {
		t.Fatalf("service ownership guard did not reject foreign contract: %v", err)
	}
	page, err := service.List(ctx, "actor", "matured", "", 20)
	if err != nil || len(page.Items) != 1 || page.Items[0].SavingsID != "own" {
		t.Fatalf("wrong owner/status list: %+v %v", page, err)
	}
}

func TestSavingsReadCacheOnlyForSettledAndRevisionBound(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2027, 10, 24, 10, 0, 0, 0, time.UTC)
	store := &testSavingsStore{revision: 8, rows: []SavingsContract{testContract("settled", "cust-1", savings.ContractSettled, now.Add(-time.Hour))}}
	cache := &testReadCache{bytes: map[string][]byte{}}
	service := &SavingsReadService{Accounts: fakeAccounts{accounts: map[string]*account.UserAccount{"actor": activePayment("actor", "cust-1", 0)}}, Store: store, Cache: cache, Now: func() time.Time { return now }}
	for i := 0; i < 2; i++ {
		c, err := service.Detail(ctx, "actor", "settled")
		if err != nil || c.Status != savings.ContractSettled {
			t.Fatalf("settled detail: %v %v", c, err)
		}
	}
	if store.reads != 1 {
		t.Fatalf("expected Redis cache hit, read count %d", store.reads)
	}
	store.revision++
	_, _ = service.Detail(ctx, "actor", "settled")
	if store.reads != 2 {
		t.Fatal("cache was reused across projection revision")
	}
}

func TestSavingsReadPaginationBoundToProjectionRevision(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2027, 10, 24, 10, 0, 0, 0, time.UTC)
	store := &testSavingsStore{revision: 8, rows: []SavingsContract{testContract("b", "cust-1", savings.ContractActive, now.Add(5*time.Hour)), testContract("a", "cust-1", savings.ContractActive, now.Add(6*time.Hour))}}
	// Distinct OpenedAt, descending order from newest to oldest.
	store.rows[0].OpenedAt = now
	store.rows[1].OpenedAt = now.Add(-time.Hour)
	service := &SavingsReadService{Accounts: fakeAccounts{accounts: map[string]*account.UserAccount{"actor": activePayment("actor", "cust-1", 0)}}, Store: store, Now: func() time.Time { return now }}
	page, err := service.List(ctx, "actor", "all", "", 1)
	if err != nil || page.NextCursor == nil || len(page.Items) != 1 {
		t.Fatalf("no cursor: %+v %v", page, err)
	}
	next, err := service.List(ctx, "actor", "all", *page.NextCursor, 1)
	if err != nil || len(next.Items) != 1 || next.Items[0].SavingsID == page.Items[0].SavingsID {
		t.Fatalf("pagination duplicate: %+v / %+v err %v", page, next, err)
	}
	store.revision++
	if _, err := service.List(ctx, "actor", "all", *page.NextCursor, 1); !errors.Is(err, ErrReadChanged) {
		t.Fatalf("stale cursor: %v", err)
	}
	if _, err := service.List(ctx, "actor", "all", "", 51); !errors.Is(err, ErrInvalidRead) {
		t.Fatalf("invalid limit: %v", err)
	}
	var response SavingsPage
	data, _ := json.Marshal(page)
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatal(err)
	}
}

// Keep the membership clock pinned to page one even when a contract matures
// between page requests and no projection event changes the checkpoint.
func TestSavingsReadPaginationKeepsInitialClassificationTime(t *testing.T) {
	ctx := context.Background()
	first := time.Date(2027, 10, 24, 9, 59, 0, 0, time.UTC)
	clock := first
	store := &testSavingsStore{revision: 7, rows: []SavingsContract{
		testContract("newer", "cust-1", savings.ContractActive, first.Add(30*time.Minute)),
		testContract("crossing", "cust-1", savings.ContractActive, first.Add(time.Minute)),
		testContract("older", "cust-1", savings.ContractActive, first.Add(time.Hour)),
	}}
	for i := range store.rows {
		store.rows[i].OpenedAt = first.Add(-time.Duration(i) * time.Hour)
	}
	service := &SavingsReadService{Accounts: fakeAccounts{accounts: map[string]*account.UserAccount{
		"actor": activePayment("actor", "cust-1", 0),
	}}, Store: store, Now: func() time.Time { return clock }}
	page, err := service.List(ctx, "actor", "active", "", 1)
	if err != nil || len(page.Items) != 1 || page.NextCursor == nil || page.Items[0].SavingsID != "newer" {
		t.Fatalf("first active page: %+v / %v", page, err)
	}
	firstCursor := *page.NextCursor
	decoded, err := DecodeSavingsCursor(firstCursor)
	if err != nil || !decoded.AsOf.Equal(first) {
		t.Fatalf("cursor lost page-one classification time: %+v / %v", decoded, err)
	}
	clock = first.Add(2 * time.Minute) // crossing contract has now matured
	page, err = service.List(ctx, "actor", "active", firstCursor, 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].SavingsID != "crossing" || page.NextCursor == nil || page.Items[0].Status != savings.ContractActive {
		t.Fatalf("second active page must reuse original time: %+v / %v", page, err)
	}
	page, err = service.List(ctx, "actor", "active", *page.NextCursor, 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].SavingsID != "older" {
		t.Fatalf("third active page skipped record: %+v / %v", page, err)
	}
	fresh, err := service.List(ctx, "actor", "matured", "", 10)
	if err != nil || len(fresh.Items) != 1 || fresh.Items[0].SavingsID != "crossing" {
		t.Fatalf("fresh page should use new classification: %+v / %v", fresh, err)
	}
	old := EncodeSavingsCursor(SavingsCursor{ID: "x", OpenedAt: first, Revision: 7, Status: "active"})
	if _, err := service.List(ctx, "actor", "active", old, 1); !errors.Is(err, ErrInvalidRead) {
		t.Fatalf("cursor without as-of must be rejected: %v", err)
	}
}

type failingSavingsReadCache struct {
	data   []byte
	found  bool
	getErr error
	setErr error
}

func (c *failingSavingsReadCache) Get(context.Context, string) ([]byte, bool, error) {
	return c.data, c.found, c.getErr
}

func (c *failingSavingsReadCache) Set(context.Context, string, []byte, time.Duration) error {
	return c.setErr
}

func TestSavingsReadCacheFailuresWarnButDoNotBlockMongo(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2027, 10, 24, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name    string
		cache   failingSavingsReadCache
		wantLog string
	}{
		{name: "GET outage", cache: failingSavingsReadCache{getErr: errors.New("redis unavailable")}, wantLog: "operation=GET"},
		{name: "SET outage", cache: failingSavingsReadCache{setErr: errors.New("redis unavailable")}, wantLog: "operation=SET"},
		{name: "bad cached JSON", cache: failingSavingsReadCache{data: []byte("{invalid"), found: true}, wantLog: "operation=DECODE"},
		{name: "normal miss", cache: failingSavingsReadCache{}, wantLog: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			store := &testSavingsStore{revision: 8, rows: []SavingsContract{testContract("settled", "cust-1", savings.ContractSettled, now.Add(-time.Hour))}}
			cache := tc.cache
			service := &SavingsReadService{
				Accounts: fakeAccounts{accounts: map[string]*account.UserAccount{"actor": activePayment("actor", "cust-1", 0)}}, Store: store, Cache: &cache,
				Logger: slog.New(slog.NewTextHandler(&logs, nil)), Now: func() time.Time { return now },
			}
			got, err := service.Detail(ctx, "actor", "settled")
			if err != nil || got.SavingsID != "settled" || store.reads != 1 {
				t.Fatalf("Mongo fallback failed: %+v / %v (reads=%d)", got, err, store.reads)
			}
			if tc.wantLog == "" && logs.Len() != 0 || tc.wantLog != "" && !strings.Contains(logs.String(), tc.wantLog) {
				t.Fatalf("unexpected cache diagnostic: %s", logs.String())
			}
			if strings.Contains(logs.String(), "cust-1") || strings.Contains(logs.String(), "savings:read:") {
				t.Fatalf("customer identifier or Redis key leaked into log: %s", logs.String())
			}
		})
	}
}
