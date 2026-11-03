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
		terms = append(terms, TermOption{TermMonths: r.Months, TermLabel: termLabel(r.Months), AnnualInterestRate: RateString(r.AnnualRateBps), AnnualRateBps: r.AnnualRateBps})
	}
	return Package{
		PackageID: StandardPackageID, Name: "Tiết kiệm có kỳ hạn", Currency: "VND",
		MinimumDeposit: savings.MinimumDepositVND, EffectiveAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), TermOptions: terms,
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
	// Independent calendar-day and HALF_UP oracles for 100M VND opened on
	// 2027-01-31 (Vietnam). Do not use CalculateMaturityPayout to compute wants.
	want := map[int]struct {
		maturity string
		interest int64
	}{
		3:  {"2027-04-30", 1_097_260},
		6:  {"2027-07-31", 2_380_274},
		9:  {"2027-10-31", 3_739_726},
		12: {"2028-01-31", 5_200_000},
		24: {"2029-01-31", 11_015_068},
		36: {"2030-01-31", 17_415_890},
		60: {"2032-01-31", 30_016_438},
	}
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
			expected, ok := want[term.Months]
			if !ok || resp.EstimatedInterest != expected.interest || resp.EstimatedTotalAtMaturity != resp.Principal+expected.interest || resp.ExpectedMaturityDate != expected.maturity {
				t.Fatalf("term=%d unexpected exact maturity/interest: got %+v, want %+v", term.Months, resp, expected)
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
	p.TermOptions = nil
	if err := ValidateCatalog([]Package{p}); !errors.Is(err, ErrPackageUnavailable) {
		t.Fatalf("package without terms accepted: %v", err)
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

func TestSavingsFuturePackageRejectedAtCatalogAndQuoteBoundary(t *testing.T) {
	now := time.Date(2026, 10, 12, 2, 10, 0, 0, time.UTC)
	future := approvedPackage()
	future.EffectiveAt = now.Add(time.Second)
	if !errors.Is(ValidateCatalogAt([]Package{future}, now), ErrPackageUnavailable) {
		t.Fatal("future-dated package passed catalog validation")
	}
	store := &fakeQuoteStore{}
	svc := &Service{
		Catalog: fakeCatalog{items: []Package{future}}, Quotes: store,
		Accounts: fakeAccounts{accounts: map[string]*account.UserAccount{
			"actor":  activePayment("actor", "customer-a", 10_000_000),
			"source": activePayment("source", "customer-a", 10_000_000),
		}}, Now: func() time.Time { return now },
	}
	_, err := svc.CreateQuote(context.Background(), "actor", CreateQuoteInput{PackageID: StandardPackageID, SourceAccountID: "source", Amount: 1_000_000, TermMonths: 3})
	if !errors.Is(err, ErrPackageUnavailable) || store.quote.ID != "" {
		t.Fatalf("future quote accepted: %v %+v", err, store.quote)
	}
	future.EffectiveAt = now
	if err := ValidateCatalogAt([]Package{future}, now); err != nil {
		t.Fatalf("package effective exactly now should be accepted: %v", err)
	}
}

func TestRateStringKeepsSecondDecimal(t *testing.T) {
	for bps, want := range map[int64]string{450: "4.5", 500: "5.0", 525: "5.25", 1205: "12.05"} {
		if got := RateString(bps); got != want {
			t.Errorf("RateString(%d) = %q, want %q", bps, got, want)
		}
	}
}

func quoteService(catalog fakeCatalog, store *fakeQuoteStore) *Service {
	return &Service{Catalog: catalog, Accounts: fakeAccounts{accounts: map[string]*account.UserAccount{
		"source": activePayment("source", "owner", 10_000_000),
	}}, Quotes: store}
}

func TestQuoteUsesPublishedRateAndVersion(t *testing.T) {
	p := approvedPackage()
	p.Version = 4
	p.TermOptions = []TermOption{{TermMonths: 12, TermLabel: "1 năm", AnnualInterestRate: "6.25", AnnualRateBps: 625}}
	store := &fakeQuoteStore{}
	resp, err := quoteService(fakeCatalog{items: []Package{p}}, store).CreateQuote(context.Background(), "source", CreateQuoteInput{
		PackageID: StandardPackageID, SourceAccountID: "source", Amount: 1_000_000, TermMonths: 12,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.AnnualInterestRate != "6.25" || store.quote.AnnualRateBps != 625 || store.quote.PackageVersion != 4 {
		t.Fatalf("resp=%+v quote=%+v", resp, store.quote)
	}
	// Kỳ hạn đã bỏ khỏi bản đang bán thì không báo giá được.
	_, err = quoteService(fakeCatalog{items: []Package{p}}, store).CreateQuote(context.Background(), "source", CreateQuoteInput{
		PackageID: StandardPackageID, SourceAccountID: "source", Amount: 1_000_000, TermMonths: 3,
	})
	if !errors.Is(err, savings.ErrUnsupportedTerm) {
		t.Fatalf("err = %v", err)
	}
	// Mức tối thiểu của gói cao hơn sàn cố định.
	p.MinimumDeposit = 5_000_000
	_, err = quoteService(fakeCatalog{items: []Package{p}}, store).CreateQuote(context.Background(), "source", CreateQuoteInput{
		PackageID: StandardPackageID, SourceAccountID: "source", Amount: 1_000_000, TermMonths: 12,
	})
	if !errors.Is(err, ErrMinimumDeposit) {
		t.Fatalf("err = %v", err)
	}
}

func TestLiveQuoteSurvivesNewVersionButNotRetirement(t *testing.T) {
	now := time.Date(2026, 10, 12, 2, 10, 0, 0, time.UTC)
	store := &fakeQuoteStore{quote: Quote{
		ID: "q", ActorID: "a", CustomerID: "c", PackageID: StandardPackageID, SourceAccountID: "s",
		Principal: 1_000_000, TermMonths: 12, AnnualRateBps: 520, ExpiresAt: now.Add(time.Minute),
	}}
	newer := approvedPackage()
	newer.TermOptions = []TermOption{{TermMonths: 12, TermLabel: "1 năm", AnnualInterestRate: "7.0", AnnualRateBps: 700}}
	svc := &Service{Catalog: fakeCatalog{items: []Package{newer}}, Quotes: store, Now: func() time.Time { return now }}
	got, err := svc.LoadValidQuote(context.Background(), "a", "q")
	if err != nil || got.AnnualRateBps != 520 {
		t.Fatalf("quote = %+v, err = %v", got, err)
	}
	svc.Catalog = fakeCatalog{items: []Package{}}
	svc.Accounts = fakeAccounts{accounts: map[string]*account.UserAccount{}}
	if _, err := svc.LoadValidQuote(context.Background(), "a", "q"); !errors.Is(err, ErrPackageRetired) {
		t.Fatalf("err = %v", err)
	}
	if _, err := svc.CreateQuote(context.Background(), "a", CreateQuoteInput{PackageID: StandardPackageID, SourceAccountID: "s", Amount: 1_000_000, TermMonths: 12}); !errors.Is(err, ErrPackageRetired) {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateCatalogRejectsMalformedRates(t *testing.T) {
	p := approvedPackage()
	p.TermOptions[0].AnnualRateBps = 0
	if err := ValidateCatalog([]Package{p}); !errors.Is(err, ErrPackageUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if err := ValidateCatalog(nil); err != nil {
		t.Fatalf("retired catalog must be valid: %v", err)
	}
}
