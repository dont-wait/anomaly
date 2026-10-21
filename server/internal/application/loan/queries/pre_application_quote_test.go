package queries

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type testLoanCatalogReader struct {
	items []LoanPackage
	err   error
	calls int
}

func (r *testLoanCatalogReader) ListActiveLoanPackages(context.Context) ([]LoanPackage, error) {
	r.calls++
	return r.items, r.err
}

func loanPackageFixture() []LoanPackage {
	return []LoanPackage{{
		PackageCode: "PERSONAL_UNSECURED_SALARY", Name: "Vay tín chấp theo lương",
		MinAmount: 1_000_000, MaxAmount: 500_000_000,
		TermMonths: []int{6, 12, 24, 36, 48, 60},
	}}
}

func quoteInputFixture() PreApplicationQuoteInput {
	return PreApplicationQuoteInput{
		LoanPackageID:   "PERSONAL_UNSECURED_SALARY",
		RequestedAmount: 50_000_000, TermMonths: 12,
		MonthlyIncome: 15_000_000, RegionCode: "KV01", ExistingMonthlyDebt: 1_000_000,
	}
}

func TestPreApplicationQuoteUsesExistingDomainPolicy(t *testing.T) {
	reader := &testLoanCatalogReader{items: loanPackageFixture()}
	service := NewPreApplicationQuoteQueryService(reader)
	fixed := time.Date(2026, 10, 21, 7, 0, 0, 0, time.UTC)
	service.Now = func() time.Time { return fixed }

	quote, err := service.Quote(context.Background(), quoteInputFixture())
	if err != nil {
		t.Fatal(err)
	}
	// 15tr/tháng ở KV01 trong code hiện tại: 15,7% + 0,4% = 16,1%.
	if quote.AnnualRateBps != 1610 {
		t.Fatalf("rate=%d, want 1610", quote.AnnualRateBps)
	}
	if quote.Official || quote.AssumedDisbursementDate != fixed || len(quote.ProvisionalSchedule) != 12 {
		t.Fatalf("unexpected estimated quote: %+v", quote)
	}
	if quote.Affordability == nil || quote.Affordability.IncomeLimit != 150_000_000 || !quote.Affordability.WithinIncomeLimit {
		t.Fatalf("wrong affordability: %+v", quote.Affordability)
	}
	var maxPayment int64
	for _, installment := range quote.ProvisionalSchedule {
		if installment.TotalPayment > maxPayment {
			maxPayment = installment.TotalPayment
		}
	}
	if quote.EstimatedMonthlyPayment != maxPayment {
		t.Fatalf("estimated monthly payment=%d, schedule max=%d", quote.EstimatedMonthlyPayment, maxPayment)
	}
	var total, interest int64
	for _, installment := range quote.ProvisionalSchedule {
		total += installment.TotalPayment
		interest += installment.InterestAmount
	}
	if quote.TotalRepayment != total || quote.TotalInterest != interest || total-interest != quote.RequestedAmount {
		t.Fatalf("invalid amounts: total=%d interest=%d principal=%d", total, interest, quote.RequestedAmount)
	}
	if reader.calls != 1 {
		t.Fatalf("called catalog %d times", reader.calls)
	}
}

func TestPreApplicationQuoteRejectsInvalidData(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*PreApplicationQuoteInput)
		want   error
	}{
		{"empty package", func(x *PreApplicationQuoteInput) { x.LoanPackageID = "" }, ErrInvalidLoanQuoteInput},
		{"unknown package", func(x *PreApplicationQuoteInput) { x.LoanPackageID = "unknown" }, ErrLoanPackageUnavailable},
		{"zero amount", func(x *PreApplicationQuoteInput) { x.RequestedAmount = 0 }, ErrInvalidLoanQuoteInput},
		{"under product minimum", func(x *PreApplicationQuoteInput) { x.RequestedAmount = 99 }, ErrInvalidLoanQuoteInput},
		{"over product maximum", func(x *PreApplicationQuoteInput) { x.RequestedAmount = 600_000_000 }, ErrInvalidLoanQuoteInput},
		{"invalid salary", func(x *PreApplicationQuoteInput) { x.MonthlyIncome = -1 }, ErrInvalidLoanQuoteInput},
		{"negative existing debt", func(x *PreApplicationQuoteInput) { x.ExistingMonthlyDebt = -1 }, ErrInvalidLoanQuoteInput},
		{"unsupported term", func(x *PreApplicationQuoteInput) { x.TermMonths = 18 }, ErrInvalidLoanQuoteInput},
		{"unsupported region", func(x *PreApplicationQuoteInput) { x.RegionCode = "KV99" }, ErrInvalidLoanQuoteInput},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			s := NewPreApplicationQuoteQueryService(&testLoanCatalogReader{items: loanPackageFixture()})
			s.Now = func() time.Time { return time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC) }
			x := quoteInputFixture()
			tt.mutate(&x)
			_, err := s.Quote(context.Background(), x)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error=%v, want %v", err, tt.want)
			}
		})
	}
}

func TestPreApplicationQuoteRequiresConfiguredActivePackage(t *testing.T) {
	s := NewPreApplicationQuoteQueryService(&testLoanCatalogReader{})
	_, err := s.Quote(context.Background(), quoteInputFixture())
	if !errors.Is(err, ErrLoanPackageUnavailable) {
		t.Fatalf("err=%v", err)
	}
	s = NewPreApplicationQuoteQueryService(&testLoanCatalogReader{err: errors.New("mongo unavailable")})
	_, err = s.Quote(context.Background(), quoteInputFixture())
	if err == nil || err.Error() != "mongo unavailable" {
		t.Fatalf("reader error=%v", err)
	}
}

func TestLoanProductCatalogReturnsEmptyArray(t *testing.T) {
	s := NewLoanProductCatalogQueryService(&testLoanCatalogReader{})
	result, err := s.List(context.Background())
	if err != nil || result == nil || len(result) != 0 {
		t.Fatalf("items=%v err=%v", result, err)
	}
}

func TestPreApplicationQuoteJSONUsesLowerCamelCase(t *testing.T) {
	q, err := NewPreApplicationQuoteQueryService(&testLoanCatalogReader{items: loanPackageFixture()}).Quote(context.Background(), quoteInputFixture())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(raw, &outer); err != nil {
		t.Fatal(err)
	}
	var affordability map[string]any
	if err := json.Unmarshal(outer["affordability"], &affordability); err != nil {
		t.Fatal(err)
	}
	if _, ok := affordability["estimatedMonthlyPayment"]; !ok {
		t.Fatalf("wrong affordability serialization: %s", raw)
	}
	var rows []map[string]any
	if err := json.Unmarshal(outer["provisionalSchedule"], &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("empty provisional schedule")
	}
	if _, ok := rows[0]["totalPayment"]; !ok {
		t.Fatalf("wrong installment serialization: %s", raw)
	}
	if strings.Contains(string(raw), "\"TotalPayment\"") || strings.Contains(string(raw), "\"IncomeLimit\"") {
		t.Fatalf("PascalCase leaked: %s", raw)
	}
}
func TestPreApplicationQuoteRejectsDuplicateActivePackage(t *testing.T) {
	dup := append(loanPackageFixture(), loanPackageFixture()...)
	svc := NewPreApplicationQuoteQueryService(&testLoanCatalogReader{items: dup})
	if _, err := svc.Quote(context.Background(), quoteInputFixture()); err == nil {
		t.Fatal("duplicate package code accepted")
	}
}
