package queries

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/savings"
)

var (
	ErrContractNotFound = errors.New("savings contract not found")
	ErrInvalidRead      = errors.New("invalid savings read request")
	ErrReadChanged      = errors.New("savings read projection changed; restart pagination")
	ErrReadNotReady     = errors.New("savings read projection unavailable")
)

const ReadCacheTTL = 15 * time.Second

type SavingsSettlement struct {
	TransactionID  string    `json:"transactionId" bson:"transaction_id"`
	InterestAmount int64     `json:"interestAmount" bson:"interest_amount"`
	TotalAmount    int64     `json:"totalAmount" bson:"total_amount"`
	SettledAt      time.Time `json:"settledAt" bson:"settled_at"`
}

type SavingsContract struct {
	SavingsID                    string                 `json:"savingsId" bson:"_id"`
	SavingsNo                    string                 `json:"savingsNo" bson:"savings_no"`
	CustomerID                   string                 `json:"-" bson:"customer_id"`
	SourceAccountID              string                 `json:"sourceAccountId" bson:"source_account_id"`
	PackageID                    string                 `json:"packageId" bson:"package_id"`
	Principal                    int64                  `json:"principal" bson:"principal"`
	TermMonths                   int                    `json:"termMonths" bson:"term_months"`
	AnnualRateBps                int64                  `json:"-" bson:"annual_rate_bps"`
	AnnualInterestRate           string                 `json:"annualInterestRate" bson:"-"`
	EarlyWithdrawalRateAtOpening *string                `json:"earlyWithdrawalRateAtOpening" bson:"-"`
	OpenedAt                     time.Time              `json:"openedAt" bson:"opened_at"`
	MaturityAt                   time.Time              `json:"maturityAt" bson:"maturity_at"`
	Status                       savings.ContractStatus `json:"status" bson:"status"`
	Settlement                   *SavingsSettlement     `json:"settlement" bson:"settlement,omitempty"`
	FinancialRevision            int64                  `json:"-" bson:"financial_revision"`
}

func (c *SavingsContract) normalize(now time.Time) error {
	if c == nil || c.SavingsID == "" || c.CustomerID == "" || c.SavingsNo == "" || c.Principal <= 0 || c.TermMonths <= 0 || c.AnnualRateBps <= 0 || c.OpenedAt.IsZero() || !c.MaturityAt.After(c.OpenedAt) || c.FinancialRevision <= 0 {
		return ErrReadNotReady
	}
	if c.Status == savings.ContractSettled && c.Settlement == nil || c.Status != savings.ContractSettled && c.Settlement != nil {
		return ErrReadNotReady
	}
	switch c.Status {
	case savings.ContractSettled, savings.ContractLocked:
	case savings.ContractActive, savings.ContractMatured:
		if !now.Before(c.MaturityAt) {
			c.Status = savings.ContractMatured
		} else {
			c.Status = savings.ContractActive
		}
	default:
		return ErrReadNotReady
	}
	c.AnnualInterestRate = RateString(c.AnnualRateBps)
	return nil
}

type SavingsHistoryItem struct {
	TransactionID string    `json:"transactionId" bson:"transaction_id"`
	Type          string    `json:"type" bson:"type"`
	Direction     string    `json:"direction" bson:"direction"`
	Amount        int64     `json:"amount" bson:"amount"`
	BalanceAfter  int64     `json:"balanceAfter" bson:"balance_after"`
	CreatedAt     time.Time `json:"createdAt" bson:"created_at"`
}

type SavingsNotice struct {
	ID         string    `json:"id" bson:"_id"`
	SavingsID  string    `json:"savingsId" bson:"savings_id"`
	SavingsNo  string    `json:"savingsNo" bson:"savings_no"`
	Type       string    `json:"type" bson:"type"`
	MaturityAt time.Time `json:"maturityAt" bson:"maturity_at"`
	CreatedAt  time.Time `json:"createdAt" bson:"created_at"`
}

type SavingsPage struct {
	Items      []SavingsContract `json:"items"`
	NextCursor *string           `json:"nextCursor"`
}

type SavingsHistory struct {
	Items []SavingsHistoryItem `json:"items"`
}
type SavingsNotices struct {
	Items []SavingsNotice `json:"items"`
}

type SavingsCursor struct {
	OpenedAt time.Time `json:"t"`
	ID       string    `json:"i"`
	Revision int64     `json:"r"`
	Status   string    `json:"s"`
	AsOf     time.Time `json:"a"`
}

func EncodeSavingsCursor(c SavingsCursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func DecodeSavingsCursor(value string) (SavingsCursor, error) {
	if len(value) > 1024 {
		return SavingsCursor{}, ErrInvalidRead
	}
	bytes, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return SavingsCursor{}, ErrInvalidRead
	}
	var c SavingsCursor
	if err := json.Unmarshal(bytes, &c); err != nil || c.OpenedAt.IsZero() || c.ID == "" || c.Revision <= 0 || c.AsOf.IsZero() {
		return SavingsCursor{}, ErrInvalidRead
	}
	return c, nil
}

type SavingsReadStore interface {
	Snapshot(context.Context) (int64, error)
	List(context.Context, string, string, int, *SavingsCursor, time.Time) ([]SavingsContract, error)
	Detail(context.Context, string, string) (*SavingsContract, error)
	History(context.Context, string, string) ([]SavingsHistoryItem, error)
	Notifications(context.Context, string, int) ([]SavingsNotice, error)
}

type SavingsReadCache interface {
	Get(context.Context, string) ([]byte, bool, error)
	Set(context.Context, string, []byte, time.Duration) error
}

type SavingsReadService struct {
	Accounts AccountReader
	Store    SavingsReadStore
	Cache    SavingsReadCache
	Logger   *slog.Logger
	Now      func() time.Time
}

func (s *SavingsReadService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *SavingsReadService) customer(ctx context.Context, actorID string) (string, error) {
	if s == nil || s.Accounts == nil || s.Store == nil || strings.TrimSpace(actorID) == "" {
		return "", ErrInvalidRead
	}
	a, err := s.Accounts.FindByID(ctx, actorID)
	if err != nil {
		return "", err
	}
	if a == nil || strings.TrimSpace(a.CustomerId) == "" {
		return "", ErrContractNotFound
	}
	return a.CustomerId, nil
}

func (s *SavingsReadService) snapshot(ctx context.Context) (int64, error) {
	r, err := s.Store.Snapshot(ctx)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrReadNotReady, err)
	}
	if r <= 0 {
		return 0, ErrReadNotReady
	}
	return r, nil
}

func (s *SavingsReadService) stable(ctx context.Context, revision int64) error {
	after, err := s.snapshot(ctx)
	if err != nil {
		return err
	}
	if after != revision {
		return ErrReadChanged
	}
	return nil
}

func (s *SavingsReadService) warnCache(ctx context.Context, op string, err error) {
	logger := s.Logger
	if logger == nil {
		logger = slog.Default()
	}
	// Redis keys can include customer/account identifiers; never log the key.
	logger.WarnContext(ctx, "savings read cache failure", "operation", op, "error", err)
}

func (s *SavingsReadService) cached(ctx context.Context, key string, dst any) bool {
	if s.Cache == nil {
		return false
	}
	b, found, err := s.Cache.Get(ctx, key)
	if err != nil {
		s.warnCache(ctx, "GET", err)
		return false
	}
	if !found {
		return false
	}
	if err := json.Unmarshal(b, dst); err != nil {
		s.warnCache(ctx, "DECODE", err)
		return false
	}
	return true
}

func (s *SavingsReadService) put(ctx context.Context, key string, value any) {
	if s.Cache == nil {
		return
	}
	b, err := json.Marshal(value)
	if err != nil {
		s.warnCache(ctx, "ENCODE", err)
		return
	}
	if err := s.Cache.Set(ctx, key, b, ReadCacheTTL); err != nil {
		s.warnCache(ctx, "SET", err)
	}
}

func (s *SavingsReadService) List(ctx context.Context, actor, status, cursor string, limit int) (SavingsPage, error) {
	if status == "" {
		status = "all"
	}
	if status != "all" && status != "active" && status != "matured" && status != "settled" {
		return SavingsPage{}, ErrInvalidRead
	}
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 50 {
		return SavingsPage{}, ErrInvalidRead
	}
	customerID, err := s.customer(ctx, actor)
	if err != nil {
		return SavingsPage{}, err
	}
	revision, err := s.snapshot(ctx)
	if err != nil {
		return SavingsPage{}, err
	}
	asOf := s.now()
	var after *SavingsCursor
	if cursor != "" {
		decoded, err := DecodeSavingsCursor(cursor)
		if err != nil || decoded.Status != status {
			return SavingsPage{}, ErrInvalidRead
		}
		if decoded.Revision != revision {
			return SavingsPage{}, ErrReadChanged
		}
		if decoded.AsOf.After(asOf) {
			return SavingsPage{}, ErrInvalidRead
		}
		asOf = decoded.AsOf
		after = &decoded
	}
	key := fmt.Sprintf("savings:read:list:%s:%s:%d:%d:%s", customerID, status, limit, revision, cursor)
	var page SavingsPage
	// Active/matured/all can change at the maturity instant without a new event.
	// Only immutable settled-only pages may be cached.
	if status == "settled" && s.cached(ctx, key, &page) {
		if err := s.stable(ctx, revision); err != nil {
			return SavingsPage{}, err
		}
		return page, nil
	}
	rows, err := s.Store.List(ctx, customerID, status, limit+1, after, asOf)
	if err != nil {
		return SavingsPage{}, err
	}
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	for i := range rows {
		if rows[i].CustomerID != customerID {
			return SavingsPage{}, ErrReadNotReady
		}
		if err := rows[i].normalize(asOf); err != nil {
			return SavingsPage{}, err
		}
	}
	page.Items = rows
	if page.Items == nil {
		page.Items = []SavingsContract{}
	}
	if more {
		last := rows[len(rows)-1]
		encoded := EncodeSavingsCursor(SavingsCursor{OpenedAt: last.OpenedAt, ID: last.SavingsID, Revision: revision, Status: status, AsOf: asOf})
		page.NextCursor = &encoded
	}
	if err := s.stable(ctx, revision); err != nil {
		return SavingsPage{}, err
	}
	if status == "settled" {
		s.put(ctx, key, page)
	}
	return page, nil
}

func (s *SavingsReadService) Detail(ctx context.Context, actor, id string) (SavingsContract, error) {
	if strings.TrimSpace(id) == "" {
		return SavingsContract{}, ErrInvalidRead
	}
	customer, err := s.customer(ctx, actor)
	if err != nil {
		return SavingsContract{}, err
	}
	revision, err := s.snapshot(ctx)
	if err != nil {
		return SavingsContract{}, err
	}
	key := fmt.Sprintf("savings:read:detail:%s:%s:%d", customer, id, revision)
	var c SavingsContract
	if s.cached(ctx, key, &c) && c.Status == savings.ContractSettled && c.SavingsID == id {
		if err := s.stable(ctx, revision); err != nil {
			return SavingsContract{}, err
		}
		return c, nil
	}
	record, err := s.Store.Detail(ctx, customer, id)
	if err != nil {
		return SavingsContract{}, err
	}
	if record == nil {
		return SavingsContract{}, ErrContractNotFound
	}
	c = *record
	if c.CustomerID != customer {
		return SavingsContract{}, ErrContractNotFound
	}
	if err := c.normalize(s.now()); err != nil {
		return SavingsContract{}, err
	}
	if err := s.stable(ctx, revision); err != nil {
		return SavingsContract{}, err
	}
	if c.Status == savings.ContractSettled {
		s.put(ctx, key, c)
	}
	return c, nil
}

func (s *SavingsReadService) History(ctx context.Context, actor, id string) (SavingsHistory, error) {
	customer, err := s.customer(ctx, actor)
	if err != nil {
		return SavingsHistory{}, err
	}
	if id == "" {
		return SavingsHistory{}, ErrInvalidRead
	}
	items, err := s.Store.History(ctx, customer, id)
	if err != nil {
		return SavingsHistory{}, err
	}
	if items == nil {
		items = []SavingsHistoryItem{}
	}
	return SavingsHistory{Items: items}, nil
}

func (s *SavingsReadService) Notifications(ctx context.Context, actor string, limit int) (SavingsNotices, error) {
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 50 {
		return SavingsNotices{}, ErrInvalidRead
	}
	customer, err := s.customer(ctx, actor)
	if err != nil {
		return SavingsNotices{}, err
	}
	items, err := s.Store.Notifications(ctx, customer, limit)
	if err != nil {
		return SavingsNotices{}, err
	}
	if items == nil {
		items = []SavingsNotice{}
	}
	return SavingsNotices{Items: items}, nil
}
