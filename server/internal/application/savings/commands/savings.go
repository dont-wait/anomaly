package commands

import (
	"context"
	"errors"
	"strings"
	"time"

	savingsqueries "github.com/dont-wait/anomaly/internal/application/savings/queries"
	"github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/domain/savings"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/google/uuid"
)

var (
	ErrConfirmationRequired         = errors.New("confirmation required")
	ErrTermsChanged                 = errors.New("savings terms have changed or are not accepted")
	ErrTermsUnavailable             = errors.New("active savings terms unavailable")
	ErrInvalidKey                   = errors.New("idempotency key must be a UUID v4")
	ErrInvalidWithdrawalQuote       = errors.New("invalid withdrawal quote")
	ErrUnsupportedWithdrawalType    = errors.New("unsupported withdrawal type")
	ErrSavingsNotSettled            = errors.New("savings contract not settled")
	ErrIdempotencyConflict          = errors.New("savings idempotency key reused for another request")
	ErrDestinationUnavailable       = errors.New("destination payment account unavailable")
	ErrWithdrawalQuoteExpired       = errors.New("withdrawal quote expired")
	ErrSavingsNotFound              = errors.New("savings contract not found")
	ErrSavingsSourceUnavailable     = errors.New("savings source account unavailable")
	ErrSavingsInsufficientFunds     = errors.New("savings insufficient funds")
	ErrSavingsWritesDisabled        = errors.New("savings event writes not enabled")
	ErrSavingsOpeningUnconfirmed    = errors.New("savings opening outcome unconfirmed")
	ErrSavingsSettlementUnconfirmed = errors.New("savings settlement outcome unconfirmed")
)

type Bank interface {
	FindByID(context.Context, string) (*account.UserAccount, error)
	OpenSavings(context.Context, journal.SavingsOpenInput) (journal.SavingsMovement, error)
	FindSavingsOpening(context.Context, string, string) (journal.SavingsMovement, bool, error)
	FindSavings(context.Context, string, string) (journal.SavingsMovement, error)
	SettleSavings(context.Context, string, string, string, string, time.Time) (journal.SavingsMovement, error)
}

type WithdrawalQuote struct {
	ID                   string    `json:"quoteId"`
	SavingsID            string    `json:"savingsId"`
	ActorID              string    `json:"actorId"`
	Principal            int64     `json:"principal"`
	Interest             int64     `json:"interestAmount"`
	Total                int64     `json:"totalPayout"`
	ExpiresAt            time.Time `json:"quoteExpiresAt"`
	MaturityAt           time.Time `json:"maturityAt"`
	DestinationAccountID string    `json:"destinationAccountId"`
}

// WithdrawalPreviewResponse exposes no JWT identity/internal store metadata.
type WithdrawalPreviewResponse struct {
	ContractAnnualInterestRate          string    `json:"contractAnnualInterestRate"`
	AppliedAnnualInterestRate           string    `json:"appliedAnnualInterestRate"`
	QuoteID                             string    `json:"quoteId"`
	SavingsID                           string    `json:"savingsId"`
	WithdrawalType                      string    `json:"withdrawalType"`
	SettlementType                      string    `json:"settlementType"`
	Principal                           int64     `json:"principal"`
	InterestAmount                      int64     `json:"interestAmount"`
	TotalPayout                         int64     `json:"totalPayout"`
	MaturityAt                          time.Time `json:"maturityAt"`
	CalculatedAt                        time.Time `json:"calculatedAt"`
	DestinationAccountID                string    `json:"destinationAccountId"`
	QuoteExpiresAt                      time.Time `json:"quoteExpiresAt"`
	RequiresEarlyWithdrawalConfirmation bool      `json:"requiresEarlyWithdrawalConfirmation"`
}

type WithdrawalQuoteStore interface {
	SaveWithdrawal(context.Context, WithdrawalQuote) error
	GetWithdrawal(context.Context, string) (WithdrawalQuote, bool, error)
}

type TermsReader interface {
	Current(context.Context) (savings.TermsSnapshot, error)
}

type Service struct {
	Terms            TermsReader
	Quotes           *savingsqueries.Service
	Bank             Bank
	WithdrawalQuotes WithdrawalQuoteStore
	Now              func() time.Time
}

func (s *Service) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func validateKey(key string) error {
	id, err := uuid.Parse(key)
	if err != nil || id.Version() != 4 || !strings.EqualFold(id.String(), key) {
		return ErrInvalidKey
	}
	return nil
}

type OpenInput struct {
	QuoteID              string `json:"quoteId"`
	SourceAccountID      string `json:"sourceAccountId"`
	Confirm              bool   `json:"confirm"`
	AcceptedTermsVersion string `json:"acceptedTermsVersion"`
}

type OpenResponse struct {
	SavingsID          string                 `json:"savingsId"`
	SavingsNo          string                 `json:"savingsNo"`
	TransactionID      string                 `json:"transactionId"`
	Status             savings.ContractStatus `json:"status"`
	Principal          int64                  `json:"principal"`
	AnnualInterestRate string                 `json:"annualInterestRate"`
	TermMonths         int                    `json:"termMonths"`
	OpenedAt           time.Time              `json:"openedAt"`
	MaturityAt         time.Time              `json:"maturityAt"`
	SourceAccountID    string                 `json:"sourceAccountId"`
}

func opened(m journal.SavingsMovement) OpenResponse {
	c := m.Contract
	return OpenResponse{SavingsID: c.ID, SavingsNo: c.SavingsNo, TransactionID: journal.EventID(c.ID, "deposit"), Status: c.Status, Principal: c.Principal, AnnualInterestRate: savingsqueries.RateString(c.AnnualRateBps), TermMonths: c.TermMonths, OpenedAt: c.OpenedAt, MaturityAt: c.MaturityAt, SourceAccountID: c.SourceAccountID}
}

func (s *Service) Open(ctx context.Context, actor, key string, in OpenInput) (OpenResponse, error) {
	if !in.Confirm {
		return OpenResponse{}, ErrConfirmationRequired
	}
	if err := validateKey(key); err != nil {
		return OpenResponse{}, err
	}
	if s == nil || s.Bank == nil || s.Quotes == nil {
		return OpenResponse{}, errors.New("savings command unavailable")
	}
	// Idempotency replay happens BEFORE quote TTL validation; a successful
	// response remains retrievable even if Redis later evicts the quote.
	old, found, err := s.Bank.FindSavingsOpening(ctx, actor, key)
	if err != nil {
		return OpenResponse{}, err
	}
	if found {
		if old.OpeningQuoteID != in.QuoteID || old.Contract.SourceAccountID != in.SourceAccountID {
			return OpenResponse{}, ErrIdempotencyConflict
		}
		return opened(old), nil
	}
	// Check consent only on NEW openings. Same-key replay always returns the already
	// committed result even if the currently published terms later change.
	if s.Terms == nil {
		return OpenResponse{}, ErrTermsUnavailable
	}
	currentTerms, termsErr := s.Terms.Current(ctx)
	if termsErr != nil || currentTerms.Validate() != nil {
		return OpenResponse{}, ErrTermsUnavailable
	}
	if strings.TrimSpace(in.AcceptedTermsVersion) == "" || in.AcceptedTermsVersion != currentTerms.Version {
		return OpenResponse{}, ErrTermsChanged
	}
	q, err := s.Quotes.LoadValidQuote(ctx, actor, in.QuoteID)
	if err != nil {
		return OpenResponse{}, err
	}
	if q.SourceAccountID != in.SourceAccountID {
		return OpenResponse{}, savingsqueries.ErrInvalidSource
	}
	m, err := s.Bank.OpenSavings(ctx, journal.SavingsOpenInput{ActorID: actor, QuoteID: q.ID, PackageID: q.PackageID, SourceAccountID: q.SourceAccountID, CustomerID: q.CustomerID, IdempotencyKey: key, Principal: q.Principal, TermMonths: q.TermMonths, AnnualRateBps: q.AnnualRateBps, ExpiresAt: q.ExpiresAt})
	if err != nil {
		return OpenResponse{}, err
	}
	return opened(m), nil
}

func (s *Service) WithdrawalPreview(ctx context.Context, actor, id, withdrawalType string) (WithdrawalPreviewResponse, error) {
	if withdrawalType != "full" {
		return WithdrawalPreviewResponse{}, ErrUnsupportedWithdrawalType
	}
	if s == nil || s.Bank == nil || s.WithdrawalQuotes == nil {
		return WithdrawalPreviewResponse{}, errors.New("withdrawal quote unavailable")
	}
	m, err := s.Bank.FindSavings(ctx, actor, id)
	if err != nil {
		return WithdrawalPreviewResponse{}, err
	}
	destination, err := s.Bank.FindByID(ctx, m.Contract.SourceAccountID)
	if err != nil {
		return WithdrawalPreviewResponse{}, err
	}
	if destination == nil || destination.Status != account.AccountStatusActive || destination.Type != account.AccountTypePayment || destination.Currency != account.CurrencyVND {
		return WithdrawalPreviewResponse{}, ErrDestinationUnavailable
	}
	now := s.now()
	if err := m.Contract.CanWithdraw(now); err != nil {
		return WithdrawalPreviewResponse{}, err
	}
	interest, total, err := savings.CalculateMaturityPayout(m.Contract.Principal, m.Contract.AnnualRateBps, m.Contract.OpenedAt, m.Contract.MaturityAt)
	if err != nil {
		return WithdrawalPreviewResponse{}, err
	}
	q := WithdrawalQuote{ID: uuid.NewString(), ActorID: actor, SavingsID: id, Principal: m.Contract.Principal, Interest: interest, Total: total, ExpiresAt: now.Add(savings.QuoteTTL), MaturityAt: m.Contract.MaturityAt, DestinationAccountID: m.Contract.SourceAccountID}
	if err := s.WithdrawalQuotes.SaveWithdrawal(ctx, q); err != nil {
		return WithdrawalPreviewResponse{}, err
	}
	// A settlement may have committed while the quote was being stored. Re-read
	// the authoritative stream before reporting success. A quote is a preview,
	// not a reservation; Withdraw must still enforce CAS before crediting.
	latest, err := s.Bank.FindSavings(ctx, actor, id)
	if err != nil {
		return WithdrawalPreviewResponse{}, err
	}
	if err := latest.Contract.CanWithdraw(s.now()); err != nil {
		return WithdrawalPreviewResponse{}, err
	}
	if latest.Contract.SourceAccountID != q.DestinationAccountID || latest.Contract.Principal != q.Principal || !latest.Contract.MaturityAt.Equal(q.MaturityAt) || latest.Contract.AnnualRateBps != m.Contract.AnnualRateBps {
		return WithdrawalPreviewResponse{}, ErrInvalidWithdrawalQuote
	}
	return WithdrawalPreviewResponse{QuoteID: q.ID, SavingsID: q.SavingsID, ContractAnnualInterestRate: savingsqueries.RateString(m.Contract.AnnualRateBps), AppliedAnnualInterestRate: savingsqueries.RateString(m.Contract.AnnualRateBps), WithdrawalType: "full", SettlementType: "at_maturity", Principal: q.Principal, InterestAmount: q.Interest, TotalPayout: q.Total, MaturityAt: q.MaturityAt, CalculatedAt: now, DestinationAccountID: q.DestinationAccountID, QuoteExpiresAt: q.ExpiresAt, RequiresEarlyWithdrawalConfirmation: false}, nil
}

type WithdrawInput struct {
	QuoteID string `json:"quoteId"`
	Confirm bool   `json:"confirm"`
}
type WithdrawResponse struct {
	SettlementType            string                 `json:"settlementType"`
	SavingsID                 string                 `json:"savingsId"`
	SavingsNo                 string                 `json:"savingsNo"`
	TransactionID             string                 `json:"transactionId"`
	Status                    savings.ContractStatus `json:"status"`
	Principal                 int64                  `json:"principal"`
	InterestAmount            int64                  `json:"interestAmount"`
	TotalPayout               int64                  `json:"totalPayout"`
	AppliedAnnualInterestRate string                 `json:"appliedAnnualInterestRate"`
	DestinationAccountID      string                 `json:"destinationAccountId"`
	SettledAt                 time.Time              `json:"settledAt"`
}

func settled(m journal.SavingsMovement) WithdrawResponse {
	c := m.Contract
	return WithdrawResponse{SettlementType: "at_maturity", SavingsID: c.ID, SavingsNo: c.SavingsNo, TransactionID: m.TransactionID, Status: c.Status, Principal: c.Principal, InterestAmount: c.Settlement.InterestAmount, TotalPayout: c.Settlement.TotalAmount, AppliedAnnualInterestRate: savingsqueries.RateString(c.AnnualRateBps), DestinationAccountID: c.SourceAccountID, SettledAt: c.Settlement.SettledAt}
}

func (s *Service) Withdraw(ctx context.Context, actor, id, key string, in WithdrawInput) (WithdrawResponse, error) {
	if !in.Confirm {
		return WithdrawResponse{}, ErrConfirmationRequired
	}
	if err := validateKey(key); err != nil {
		return WithdrawResponse{}, err
	}
	if s == nil || s.Bank == nil || s.WithdrawalQuotes == nil {
		return WithdrawResponse{}, errors.New("savings command unavailable")
	}
	// Successful duplicate requests may be retried after quote expiry.
	current, err := s.Bank.FindSavings(ctx, actor, id)
	if err != nil {
		return WithdrawResponse{}, err
	}
	if current.Contract.Status == savings.ContractSettled {
		if current.WithdrawalKey == actor+":"+strings.ToLower(key) {
			if current.WithdrawalQuoteID != in.QuoteID {
				return WithdrawResponse{}, ErrIdempotencyConflict
			}
			// Retry the identical request even if its quote has expired or
			// disappeared from the quote cache.
			return settled(current), nil
		}
		return WithdrawResponse{}, savings.ErrAlreadySettled
	}
	q, found, err := s.WithdrawalQuotes.GetWithdrawal(ctx, in.QuoteID)
	if err != nil {
		return WithdrawResponse{}, err
	}
	if !found {
		return WithdrawResponse{}, savingsqueries.ErrQuoteNotFound
	}
	if !s.now().Before(q.ExpiresAt) {
		return WithdrawResponse{}, ErrWithdrawalQuoteExpired
	}
	if q.ActorID != actor || q.SavingsID != id || q.DestinationAccountID != current.Contract.SourceAccountID || q.Principal != current.Contract.Principal || !q.MaturityAt.Equal(current.Contract.MaturityAt) {
		return WithdrawResponse{}, ErrInvalidWithdrawalQuote
	}
	interest, total, err := savings.CalculateMaturityPayout(current.Contract.Principal, current.Contract.AnnualRateBps, current.Contract.OpenedAt, current.Contract.MaturityAt)
	if err != nil {
		return WithdrawResponse{}, err
	}
	if interest != q.Interest || total != q.Total {
		return WithdrawResponse{}, ErrInvalidWithdrawalQuote
	}
	m, err := s.Bank.SettleSavings(ctx, actor, id, q.ID, key, q.ExpiresAt)
	if err != nil {
		return WithdrawResponse{}, err
	}
	return settled(m), nil
}

type SettlementDetails struct {
	Type           string    `json:"type"`
	Principal      int64     `json:"principal"`
	InterestAmount int64     `json:"interestAmount"`
	TotalAmount    int64     `json:"totalAmount"`
	TransactionID  string    `json:"transactionId"`
	SettledAt      time.Time `json:"settledAt"`
}
type SettlementReceipt struct {
	SavingsID  string                 `json:"savingsId"`
	SavingsNo  string                 `json:"savingsNo"`
	Status     savings.ContractStatus `json:"status"`
	Settlement SettlementDetails      `json:"settlement"`
}

func (s *Service) Receipt(ctx context.Context, actor, id string) (SettlementReceipt, error) {
	if s == nil || s.Bank == nil {
		return SettlementReceipt{}, errors.New("savings command unavailable")
	}
	m, err := s.Bank.FindSavings(ctx, actor, id)
	if err != nil {
		return SettlementReceipt{}, err
	}
	c := m.Contract
	if c.Status != savings.ContractSettled || c.Settlement == nil {
		return SettlementReceipt{}, ErrSavingsNotSettled
	}
	return SettlementReceipt{SavingsID: c.ID, SavingsNo: c.SavingsNo, Status: c.Status, Settlement: SettlementDetails{Type: "at_maturity", Principal: c.Principal, InterestAmount: c.Settlement.InterestAmount, TotalAmount: c.Settlement.TotalAmount, TransactionID: c.Settlement.TransactionID, SettledAt: c.Settlement.SettledAt}}, nil
}
