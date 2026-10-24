package savings

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/timeutil"
)

func localDate(year int, month time.Month, day, hour, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, timeutil.VietnamLocation)
}

func TestCurrentRatesAndMinimumDeposit(t *testing.T) {
	want := []TermRate{{3, 450}, {6, 480}, {9, 500}, {12, 520}, {24, 550}, {36, 580}, {60, 600}}
	got := CurrentTermRates()
	if len(got) != len(want) {
		t.Fatalf("got %d terms, want %d", len(got), len(want))
	}
	for i, term := range want {
		if got[i] != term {
			t.Fatalf("term[%d] = %+v, want %+v", i, got[i], term)
		}
		bps, err := AnnualRateBpsForTerm(term.Months)
		if err != nil || bps != term.AnnualRateBps {
			t.Fatalf("rate for %d months: %d, %v", term.Months, bps, err)
		}
		if err := ValidateDeposit(MinimumDepositVND, term.Months); err != nil {
			t.Fatalf("minimum principal for %d months rejected: %v", term.Months, err)
		}
	}
	got[0].AnnualRateBps = 1 // caller cannot change the policy
	if CurrentTermRates()[0].AnnualRateBps != 450 {
		t.Fatal("CurrentTermRates returned shared mutable state")
	}
	if err := ValidateDeposit(999_999, 3); !errors.Is(err, ErrInvalidDepositAmount) {
		t.Fatalf("expected invalid amount, got %v", err)
	}
	if err := ValidateDeposit(10_000_000_000, 60); err != nil {
		t.Fatalf("there must be no fixed max deposit: %v", err)
	}
	for _, months := range []int{-1, 0, 1, 2, 4, 18, 61} {
		if err := ValidateDeposit(MinimumDepositVND, months); !errors.Is(err, ErrUnsupportedTerm) {
			t.Fatalf("term=%d: expected unsupported term; got %v", months, err)
		}
	}
}

func TestCalculateMaturityAtCalendarMonthClampAndClock(t *testing.T) {
	tests := []struct {
		name   string
		start  time.Time
		months int
		want   time.Time
	}{
		{"ordinary", localDate(2027, 1, 15, 14, 30), 3, localDate(2027, 4, 15, 14, 30)},
		{"clamp to April 30", localDate(2027, 1, 31, 14, 30), 3, localDate(2027, 4, 30, 14, 30)},
		{"not sticky end of month", localDate(2027, 2, 28, 14, 30), 3, localDate(2027, 5, 28, 14, 30)},
		{"February non leap", localDate(2026, 11, 30, 8, 15), 3, localDate(2027, 2, 28, 8, 15)},
		{"February leap", localDate(2023, 11, 30, 8, 15), 3, localDate(2024, 2, 29, 8, 15)},
		{"five years", localDate(2024, 2, 29, 10, 0), 60, localDate(2029, 2, 28, 10, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculateMaturityAt(tt.start, tt.months)
			if err != nil || !got.Equal(tt.want) || got.Location().String() != timeutil.VietnamLocation.String() {
				t.Fatalf("got %v (%v), want %v", got, err, tt.want)
			}
		})
	}
	if _, err := CalculateMaturityAt(time.Time{}, 3); !errors.Is(err, ErrInvalidOpenedAt) {
		t.Fatalf("missing date accepted: %v", err)
	}
}

func TestInterestActualDaysHalfUpAndFixedSnapshot(t *testing.T) {
	start := localDate(2026, 1, 1, 14, 30)
	end := localDate(2026, 4, 1, 14, 30)
	days, err := ActualCalendarDays(start, end)
	if err != nil || days != 90 {
		t.Fatalf("got days=%d err=%v, want 90", days, err)
	}
	interest, total, err := CalculateMaturityPayout(1_000_000, 450, start, end)
	if err != nil || interest != 11_096 || total != 1_011_096 {
		t.Fatalf("got interest=%d total=%d err=%v", interest, total, err)
	}
	start = localDate(2026, 10, 12, 9, 10)
	end = localDate(2027, 10, 12, 9, 10)
	interest, total, err = CalculateMaturityPayout(100_000_000, 520, start, end)
	if err != nil || interest != 5_200_000 || total != 105_200_000 {
		t.Fatalf("got interest=%d total=%d err=%v", interest, total, err)
	}
	// Rate is a contract snapshot: no query to the current product rate.
	interest, err = CalculateMaturityInterest(100_000_000, 500, start, end)
	if err != nil || interest != 5_000_000 {
		t.Fatalf("snapshot rate was overwritten: %d %v", interest, err)
	}
	// Exactly 0.5 VND: 1_825_000 * 1 bps * 1 day / 365.
	interest, err = CalculateMaturityInterest(1_825_000, 1,
		localDate(2027, 1, 1, 12, 0), localDate(2027, 1, 2, 12, 0))
	if err != nil || interest != 1 {
		t.Fatalf("half-up tie: got %d, %v; want 1", interest, err)
	}
	leapStart := localDate(2023, 3, 1, 10, 0)
	leapEnd := localDate(2024, 3, 1, 10, 0)
	days, err = ActualCalendarDays(leapStart, leapEnd)
	if err != nil || days != 366 {
		t.Fatalf("leap days=%d err=%v", days, err)
	}
}

func TestInterestRejectsInvalidInputAndOverflow(t *testing.T) {
	start := localDate(2027, 1, 1, 10, 0)
	end := localDate(2027, 4, 1, 10, 0)
	for _, tc := range []struct {
		principal, rate int64
		start, end      time.Time
		want            error
	}{
		{999_999, 450, start, end, ErrInvalidDepositAmount},
		{1_000_000, 0, start, end, ErrInvalidInterestRate},
		{1_000_000, 10_001, start, end, ErrInvalidInterestRate},
		{1_000_000, 450, start, start, ErrInvalidMaturity},
		{1_000_000, 450, time.Time{}, end, ErrInvalidOpenedAt},
	} {
		_, err := CalculateMaturityInterest(tc.principal, tc.rate, tc.start, tc.end)
		if !errors.Is(err, tc.want) {
			t.Fatalf("expected %v, got %v", tc.want, err)
		}
	}
	if _, _, err := CalculateMaturityPayout(math.MaxInt64, 600, start, end); !errors.Is(err, ErrAmountOverflow) {
		t.Fatalf("want overflow error, got %v", err)
	}
}

func testContractParams() OpenContractParams {
	return OpenContractParams{
		ID: "id_1", SavingsNo: "SV001", CustomerID: "c1", PackageID: "savings_standard",
		SourceAccountID: "acc1", Principal: 100_000_000, TermMonths: 12,
		AnnualRateBps: 520, OpenedAt: localDate(2026, 10, 12, 9, 10),
	}
}

func TestContractMaturityAndManualWithdrawal(t *testing.T) {
	c, err := NewContract(testContractParams())
	if err != nil || c.Status != ContractActive || c.Settlement != nil {
		t.Fatalf("invalid new savings contract: %+v err=%v", c, err)
	}
	if err := c.CanWithdraw(c.MaturityAt.Add(-time.Nanosecond)); !errors.Is(err, ErrNotMatured) {
		t.Fatalf("early withdrawal accepted: %v", err)
	}
	// Customer may withdraw even if scheduler has not changed status yet.
	if err := c.CanWithdraw(c.MaturityAt); err != nil {
		t.Fatalf("customer must not wait for scheduler: %v", err)
	}
	if c.Status != ContractActive {
		t.Fatal("eligibility check must not mutate state or auto-credit account")
	}
	if err := c.MarkMatured(c.MaturityAt.Add(time.Minute)); err != nil || c.Status != ContractMatured {
		t.Fatalf("maturity transition failed: %v", err)
	}
	// Waiting any amount of time does not add post-maturity interest.
	if err := c.ConfirmSettled("tx_1", c.MaturityAt.Add(30*24*time.Hour)); err != nil {
		t.Fatalf("confirm settlement failed: %v", err)
	}
	if c.Status != ContractSettled || c.Settlement.InterestAmount != 5_200_000 ||
		c.Settlement.TotalAmount != 105_200_000 || c.Settlement.TransactionID != "tx_1" {
		t.Fatalf("wrong settlement: %+v", c)
	}
	if err := c.ConfirmSettled("tx_2", c.MaturityAt.Add(31*24*time.Hour)); !errors.Is(err, ErrAlreadySettled) {
		t.Fatalf("duplicate withdrawal accepted: %v", err)
	}
	if err := c.MarkMatured(c.MaturityAt.Add(31 * 24 * time.Hour)); !errors.Is(err, ErrAlreadySettled) {
		t.Fatalf("settled contract was matured again: %v", err)
	}
}

func TestContractInvalidAndLocked(t *testing.T) {
	p := testContractParams()
	p.SourceAccountID = " "
	if _, err := NewContract(p); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("invalid source accepted: %v", err)
	}
	p = testContractParams()
	p.AnnualRateBps = 0
	if _, err := NewContract(p); !errors.Is(err, ErrInvalidInterestRate) {
		t.Fatalf("invalid rate accepted: %v", err)
	}
	c, err := NewContract(testContractParams())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ConfirmSettled("", c.MaturityAt); !errors.Is(err, ErrInvalidSettlement) {
		t.Fatalf("empty bank transaction accepted: %v", err)
	}
	if c.Settlement != nil || c.Status != ContractActive {
		t.Fatal("failed settlement must leave contract unchanged")
	}
	c.Status = ContractLocked
	if err := c.CanWithdraw(c.MaturityAt); !errors.Is(err, ErrContractLocked) {
		t.Fatalf("locked savings can be withdrawn: %v", err)
	}
}
