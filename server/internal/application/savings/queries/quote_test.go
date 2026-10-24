package queries

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/domain/savings"
)

type fakeCatalog struct {
	items []Package
	err   error
}

func (f fakeCatalog) ListActive(context.Context) ([]Package, error) { return f.items, f.err }

type fakeAccounts struct {
	accounts map[string]*account.UserAccount
}

func (f fakeAccounts) FindByID(_ context.Context, id string) (*account.UserAccount, error) {
	return f.accounts[id], nil
}

type fakeQuoteStore struct {
	quote Quote
	err   error
}

func (f *fakeQuoteStore) Save(_ context.Context, q Quote) error {
	if f.err != nil {
		return f.err
	}
	f.quote = q
	return nil
}

func (f *fakeQuoteStore) Get(_ context.Context, id string) (Quote, bool, error) {
	return f.quote, id == f.quote.ID, nil
}

func approvedPackage() Package {
	terms := []TermOption{}
	for _, r := range savings.CurrentTermRates() {
		terms = append(terms, TermOption{TermMonths: r.Months, TermLabel: termLabel(r.Months), AnnualInterestRate: RateString(r.AnnualRateBps)})
	}
	return Package{
		PackageID: StandardPackageID, Name: "Tiết kiệm có kỳ hạn", Currency: "VND",
		MinimumDeposit: savings.MinimumDepositVND, EffectiveAt: time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC), TermOptions: terms,
	}
}

func activePayment(id, customer string, funds int64) *account.UserAccount {
	return &account.UserAccount{
		Id: id, CustomerId: customer, Status: account.AccountStatusActive,
		Type: account.AccountTypePayment, Currency: account.CurrencyVND,
		Balance: account.Balance{Current: funds},
	}
}

func TestCreateQuoteAllApprovedTerms(t *testing.T) {
	now := time.Date(2027, 1, 31, 7, 30, 0, 0, time.UTC) // 14:30 Vietnam
	for _, term := range savings.CurrentTermRates() {
		t.Run(termLabel(term.Months), func(t *testing.T) {
			store := &fakeQuoteStore{}
			service := &Service{
				Catalog: fakeCatalog{items: []Package{approvedPackage()}},
				Accounts: fakeAccounts{accounts: map[string]*account.UserAccount{
					"actor":  activePayment("actor", "customer-a", 150_000_000),
					"source": activePayment("source", "customer-a", 150_000_000),
				}},
				Quotes: store, Now: func() time.Time { return now },
			}
			resp, err := service.CreateQuote(context.Background(), "actor", CreateQuoteInput{
				PackageID: StandardPackageID, SourceAccountID: "source",
				Amount: 100_000_000, TermMonths: term.Months,
			})
			if err != nil {
				t.Fatal(err)
			}
			if resp.AnnualInterestRate != RateString(term.AnnualRateBps) || resp.QuoteID == "" || resp.IsFinal {
				t.Fatalf("invalid rate or quote response: %+v", resp)
			}
			if resp.QuoteExpiresAt.Sub(now) != 5*time.Minute || store.quote.ActorID != "actor" || store.quote.CustomerID != "customer-a" {
				t.Fatalf("invalid bound quote: %+v", store.quote)
			}
			if resp.EstimatedInterest <= 0 || resp.EstimatedTotalAtMaturity != resp.Principal+resp.EstimatedInterest {
				t.Fatalf("invalid interest: %+v", resp)
			}
			if term.Months == 3 && resp.ExpectedMaturityDate != "2027-04-30" {
				t.Fatalf("Jan 31 maturity = %q, want 2027-04-30", resp.ExpectedMaturityDate)
			}
		})
	}
}

func TestQuoteRejectsInvalidInputs(t *testing.T) {
	now := time.Date(2026, 10, 12, 2, 10, 0, 0, time.UTC)
	cases := []struct {
		name   string
		actor  string
		source string
		amount int64
		term   int
		pkg    string
		want   error
	}{
		{"not owned", "actor", "foreign", 1_000_000, 3, StandardPackageID, ErrInvalidSource},
		{"missing actor", "missing", "source", 1_000_000, 3, StandardPackageID, ErrInvalidSource},
		{"empty source", "actor", "", 1_000_000, 3, StandardPackageID, ErrInvalidSource},
		{"not enough", "actor", "source", 200_000_000, 3, StandardPackageID, ErrInsufficientFunds},
		{"below minimum", "actor", "source", 999_999, 3, StandardPackageID, ErrMinimumDeposit},
		{"nonpositive", "actor", "source", 0, 3, StandardPackageID, ErrInvalidAmount},
		{"not supported", "actor", "source", 1_000_000, 1, StandardPackageID, savings.ErrUnsupportedTerm},
		{"unknown package", "actor", "source", 1_000_000, 3, "other", ErrPackageUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeQuoteStore{}
			svc := &Service{
				Catalog: fakeCatalog{items: []Package{approvedPackage()}}, Quotes: store,
				Accounts: fakeAccounts{accounts: map[string]*account.UserAccount{
					"actor":   activePayment("actor", "customer-a", 150_000_000),
					"source":  activePayment("source", "customer-a", 150_000_000),
					"foreign": activePayment("foreign", "customer-b", 150_000_000),
				}}, Now: func() time.Time { return now },
			}
			_, err := svc.CreateQuote(context.Background(), tc.actor, CreateQuoteInput{
				SourceAccountID: tc.source, PackageID: tc.pkg, Amount: tc.amount, TermMonths: tc.term,
			})
			if !errors.Is(err, tc.want) || store.quote.ID != "" {
				t.Fatalf("err=%v want=%v; stored=%+v", err, tc.want, store.quote)
			}
		})
	}
}

func TestQuoteRejectsInactiveAccountsAndUnapprovedCatalog(t *testing.T) {
	p := approvedPackage()
	p.TermOptions[0].AnnualInterestRate = "9.0"
	if err := ValidateCatalog([]Package{p}); !errors.Is(err, ErrPackageUnavailable) {
		t.Fatalf("catalog mismatch accepted: %v", err)
	}
	p = approvedPackage()
	p.TermOptions = p.TermOptions[1:]
	if err := ValidateCatalog([]Package{p}); !errors.Is(err, ErrPackageUnavailable) {
		t.Fatalf("missing term accepted: %v", err)
	}
	p = approvedPackage()
	acc := activePayment("source", "owner", 2_000_000)
	acc.Status = "locked"
	svc := &Service{Catalog: fakeCatalog{items: []Package{p}}, Accounts: fakeAccounts{accounts: map[string]*account.UserAccount{
		"source": acc,
	}}, Quotes: &fakeQuoteStore{}}
	_, err := svc.CreateQuote(context.Background(), "source", CreateQuoteInput{
		PackageID: StandardPackageID, SourceAccountID: "source", Amount: 1_000_000, TermMonths: 3,
	})
	if !errors.Is(err, ErrInvalidSource) {
		t.Fatalf("locked account accepted: %v", err)
	}
}

func TestLoadValidQuoteRejectsExpiryAndAnotherActor(t *testing.T) {
	now := time.Date(2026, 10, 12, 2, 10, 0, 0, time.UTC)
	store := &fakeQuoteStore{}
	svc := &Service{
		Catalog:  fakeCatalog{items: []Package{approvedPackage()}},
		Accounts: fakeAccounts{accounts: map[string]*account.UserAccount{"owner": activePayment("owner", "customer", 2_000_000)}},
		Quotes:   store, Now: func() time.Time { return now },
	}
	response, err := svc.CreateQuote(context.Background(), "owner", CreateQuoteInput{
		PackageID: StandardPackageID, SourceAccountID: "owner", Amount: 1_000_000, TermMonths: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LoadValidQuote(context.Background(), "stranger", response.QuoteID); !errors.Is(err, ErrQuoteInvalid) {
		t.Fatalf("foreign actor loaded quote: %v", err)
	}
	if _, err := svc.LoadValidQuote(context.Background(), "owner", response.QuoteID); err != nil {
		t.Fatalf("valid quote not loaded: %v", err)
	}
	svc.Now = func() time.Time { return now.Add(5 * time.Minute) }
	if _, err := svc.LoadValidQuote(context.Background(), "owner", response.QuoteID); !errors.Is(err, ErrQuoteExpired) {
		t.Fatalf("expired quote loaded: %v", err)
	}
}
