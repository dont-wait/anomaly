package queries

import (
	"context"
	"encoding/json"
	"errors"
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
		if after != nil && !c.OpenedAt.Before(after.OpenedAt) && !(c.OpenedAt.Equal(after.OpenedAt) && c.SavingsID < after.ID) {
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

func (s *testSavingsStore) Detail(_ context.Context, customer, id string) (*SavingsContract, error) {
	s.reads++
	for _, c := range s.rows {
		if c.CustomerID == customer && c.SavingsID == id {
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
		t.Fatalf("foreign accessible: %v", err)
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
