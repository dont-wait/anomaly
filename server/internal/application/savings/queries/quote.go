package queries

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/domain/savings"
	"github.com/google/uuid"
)

const StandardPackageID = "savings_standard"

var (
	ErrPackageUnavailable = errors.New("savings package not found or inactive")
	ErrInvalidAmount      = errors.New("invalid savings amount")
	ErrMinimumDeposit     = errors.New("amount is below minimum deposit")
	ErrInvalidSource      = errors.New("invalid or non-owned payment account")
	ErrInsufficientFunds  = errors.New("insufficient funds")
	ErrQuoteInvalid       = errors.New("invalid savings quote")
	ErrQuoteNotFound      = errors.New("savings quote not found")
	ErrQuoteExpired       = errors.New("savings quote expired")
)

type TermOption struct {
	TermMonths         int    `json:"termMonths"`
	TermLabel          string `json:"termLabel"`
	AnnualInterestRate string `json:"annualInterestRate"`
}

type Package struct {
	PackageID      string       `json:"packageId"`
	Name           string       `json:"name"`
	Currency       string       `json:"currency"`
	TermOptions    []TermOption `json:"termOptions"`
	MinimumDeposit int64        `json:"minimumDeposit"`
	EffectiveAt    time.Time    `json:"effectiveAt"`
}

type Catalog interface {
	ListActive(ctx context.Context) ([]Package, error)
}

type AccountReader interface {
	FindByID(ctx context.Context, id string) (*account.UserAccount, error)
}

// Quote is stored server-side and bound to the JWT actor and source account.
// An opening command (GĐ3) MUST load and revalidate it, never trust client-side
// principal/rate, and must calculate maturityAt from the actual commit time.
type Quote struct {
	ID              string    `json:"id"`
	ActorID         string    `json:"actorId"`
	CustomerID      string    `json:"customerId"`
	PackageID       string    `json:"packageId"`
	SourceAccountID string    `json:"sourceAccountId"`
	Principal       int64     `json:"principal"`
	TermMonths      int       `json:"termMonths"`
	AnnualRateBps   int64     `json:"annualRateBps"`
	QuotedAt        time.Time `json:"quotedAt"`
	ExpiresAt       time.Time `json:"expiresAt"`
}

type QuoteStore interface {
	Save(ctx context.Context, quote Quote) error
	Get(ctx context.Context, id string) (Quote, bool, error)
}

type CreateQuoteInput struct {
	PackageID       string `json:"packageId"`
	SourceAccountID string `json:"sourceAccountId"`
	Amount          int64  `json:"amount"`
	TermMonths      int    `json:"termMonths"`
}

type QuoteResponse struct {
	QuoteID                  string    `json:"quoteId"`
	Principal                int64     `json:"principal"`
	Currency                 string    `json:"currency"`
	TermMonths               int       `json:"termMonths"`
	AnnualInterestRate       string    `json:"annualInterestRate"`
	ExpectedMaturityDate     string    `json:"expectedMaturityDate"`
	EstimatedInterest        int64     `json:"estimatedInterest"`
	EstimatedTotalAtMaturity int64     `json:"estimatedTotalAtMaturity"`
	QuoteExpiresAt           time.Time `json:"quoteExpiresAt"`
	IsFinal                  bool      `json:"isFinal"`
}

type Service struct {
	Catalog  Catalog
	Accounts AccountReader
	Quotes   QuoteStore
	Now      func() time.Time
}

func RateString(bps int64) string {
	return fmt.Sprintf("%d.%d", bps/100, (bps%100)/10)
}

func termLabel(months int) string {
	if months%12 == 0 {
		return fmt.Sprintf("%d năm", months/12)
	}
	return fmt.Sprintf("%d tháng", months)
}

// ValidateCatalog fails closed if stored Mongo policy and approved domain policy
// diverge; the quote must never use a term/rate the domain won't accept.
func ValidateCatalog(items []Package) error {
	return ValidateCatalogAt(items, time.Now().UTC())
}

// ValidateCatalogAt evaluates product eligibility at a single authoritative clock instant.
func ValidateCatalogAt(items []Package, now time.Time) error {
	if len(items) != 1 {
		return ErrPackageUnavailable
	}
	p := items[0]
	if p.PackageID != StandardPackageID || strings.TrimSpace(p.Name) == "" || p.Currency != "VND" || p.MinimumDeposit != savings.MinimumDepositVND || (p.EffectiveAt.IsZero() || p.EffectiveAt.After(now)) {
		return ErrPackageUnavailable
	}
	approved := savings.CurrentTermRates()
	if len(p.TermOptions) != len(approved) {
		return ErrPackageUnavailable
	}
	for i, rate := range approved {
		t := p.TermOptions[i]
		if t.TermMonths != rate.Months || t.AnnualInterestRate != RateString(rate.AnnualRateBps) || t.TermLabel != termLabel(rate.Months) {
			return ErrPackageUnavailable
		}
	}
	return nil
}

func (s *Service) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) ListPackages(ctx context.Context) ([]Package, error) {
	if s == nil || s.Catalog == nil {
		return nil, errors.New("savings catalog unavailable")
	}
	items, err := s.Catalog.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	if err := ValidateCatalogAt(items, s.now()); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Service) CreateQuote(ctx context.Context, actorID string, in CreateQuoteInput) (QuoteResponse, error) {
	if s == nil || s.Accounts == nil || s.Quotes == nil {
		return QuoteResponse{}, errors.New("savings quote service unavailable")
	}
	if strings.TrimSpace(actorID) == "" || strings.TrimSpace(in.SourceAccountID) == "" {
		return QuoteResponse{}, ErrInvalidSource
	}
	if in.Amount < savings.MinimumDepositVND {
		if in.Amount <= 0 {
			return QuoteResponse{}, ErrInvalidAmount
		}
		return QuoteResponse{}, ErrMinimumDeposit
	}
	if _, err := savings.AnnualRateBpsForTerm(in.TermMonths); err != nil {
		return QuoteResponse{}, err
	}
	items, err := s.ListPackages(ctx)
	if err != nil {
		return QuoteResponse{}, err
	}
	if in.PackageID != items[0].PackageID {
		return QuoteResponse{}, ErrPackageUnavailable
	}
	actor, err := s.Accounts.FindByID(ctx, actorID)
	if err != nil {
		return QuoteResponse{}, err
	}
	if actor == nil || actor.Status != account.AccountStatusActive {
		return QuoteResponse{}, ErrInvalidSource
	}
	ownerAccount, err := s.Accounts.FindByID(ctx, in.SourceAccountID)
	if err != nil {
		return QuoteResponse{}, err
	}
	if ownerAccount == nil || ownerAccount.Status != account.AccountStatusActive || ownerAccount.Type != account.AccountTypePayment || ownerAccount.Currency != account.CurrencyVND {
		return QuoteResponse{}, ErrInvalidSource
	}
	if actor.Id != ownerAccount.Id && (actor.CustomerId == "" || actor.CustomerId != ownerAccount.CustomerId) {
		return QuoteResponse{}, ErrInvalidSource
	}
	if ownerAccount.CustomerId == "" {
		return QuoteResponse{}, ErrInvalidSource
	}
	if ownerAccount.Balance.Current < in.Amount {
		return QuoteResponse{}, ErrInsufficientFunds
	}
	now := s.now()
	maturity, err := savings.CalculateMaturityAt(now, in.TermMonths)
	if err != nil {
		return QuoteResponse{}, err
	}
	rate, _ := savings.AnnualRateBpsForTerm(in.TermMonths)
	interest, total, err := savings.CalculateMaturityPayout(in.Amount, rate, now, maturity)
	if err != nil {
		return QuoteResponse{}, err
	}
	q := Quote{
		ID: uuid.NewString(), ActorID: actorID, CustomerID: ownerAccount.CustomerId,
		PackageID: in.PackageID, SourceAccountID: ownerAccount.Id, Principal: in.Amount,
		TermMonths: in.TermMonths, AnnualRateBps: rate, QuotedAt: now,
		ExpiresAt: now.Add(savings.QuoteTTL),
	}
	if err := s.Quotes.Save(ctx, q); err != nil {
		return QuoteResponse{}, err
	}
	return QuoteResponse{
		QuoteID: q.ID, Principal: in.Amount, Currency: "VND", TermMonths: in.TermMonths,
		AnnualInterestRate: RateString(rate), ExpectedMaturityDate: maturity.Format("2006-01-02"),
		EstimatedInterest: interest, EstimatedTotalAtMaturity: total,
		QuoteExpiresAt: q.ExpiresAt, IsFinal: false,
	}, nil
}

// LoadValidQuote is an authority boundary for the GĐ3 opening command. It
// does not spend funds; GĐ3 MUST still reload the bank account and verify the
// quoted policy before appending the money-moving events.
func (s *Service) LoadValidQuote(ctx context.Context, actorID, quoteID string) (Quote, error) {
	if s == nil || s.Quotes == nil || strings.TrimSpace(actorID) == "" || strings.TrimSpace(quoteID) == "" {
		return Quote{}, ErrQuoteInvalid
	}
	q, found, err := s.Quotes.Get(ctx, quoteID)
	if err != nil {
		return Quote{}, err
	}
	if !found {
		return Quote{}, ErrQuoteNotFound
	}
	clock := time.Now
	if s.Now != nil {
		clock = s.Now
	}
	if !clock().UTC().Before(q.ExpiresAt) {
		return Quote{}, ErrQuoteExpired
	}
	if q.ActorID != actorID || q.ID != quoteID || q.CustomerID == "" || q.SourceAccountID == "" || q.Principal < savings.MinimumDepositVND {
		return Quote{}, ErrQuoteInvalid
	}
	rate, err := savings.AnnualRateBpsForTerm(q.TermMonths)
	if err != nil || rate != q.AnnualRateBps {
		return Quote{}, ErrQuoteInvalid
	}
	items, err := s.ListPackages(ctx)
	if err != nil {
		return Quote{}, err
	}
	if q.PackageID != items[0].PackageID {
		return Quote{}, ErrQuoteInvalid
	}
	return q, nil
}
