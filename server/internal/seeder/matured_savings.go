package seeder

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/domain/savings"
	journal "github.com/dont-wait/anomaly/internal/domain/transaction"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	"github.com/dont-wait/anomaly/internal/logger"
	"github.com/dont-wait/anomaly/internal/timeutil"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

func init() {
	seeders.register("030_matured_savings", seedMaturedSavings)
}

const (
	maturedSavingsPackageID = "savings_standard"
	maturedSavingsTerm      = 3
	maturedSavingsPrincipal = 10_000_000
)

var maturedSavingsNamespace = uuid.MustParse("6f1c2a52-7d0e-4b8f-9a51-3c0d2b7e9f14")

type SavingsSeeder interface {
	SeedBackdatedSavings(context.Context, eventstore.SeedSavingsInput) (journal.SavingsMovement, bool, error)
}

// seedMaturedSavings opens a savings contract that has matured by now for
// the account whose CCCD is in SEED_MATURED_SAVINGS_CCCD. It does nothing when
// the variable is unset, so real identities never live in the repository.
// Repeated runs reuse that day's active contract, but create a fresh contract
// if the previous demo contract has already been settled.
func seedMaturedSavings(ctx context.Context, deps Dependencies) error {
	cccd := strings.TrimSpace(os.Getenv("SEED_MATURED_SAVINGS_CCCD"))
	if cccd == "" || deps.Savings == nil {
		return nil
	}
	account, err := deps.Accounts.FindByCCCDNumber(ctx, cccd)
	if err != nil {
		return fmt.Errorf("find account by CCCD: %w", err)
	}
	if account == nil {
		return fmt.Errorf("no account with CCCD %s", cccd)
	}
	now := time.Now().In(timeutil.VietnamLocation)
	openedAt, err := maturedTodayOpening(now, maturedSavingsTerm)
	if err != nil {
		return err
	}
	rate, err := savings.AnnualRateBpsForTerm(maturedSavingsTerm)
	if err != nil {
		return err
	}
	day := now.Format(time.DateOnly)
	m, created, err := deps.Savings.SeedBackdatedSavings(ctx, eventstore.SeedSavingsInput{
		AccountID:      account.Id,
		PackageID:      maturedSavingsPackageID,
		PackageVersion: 1,
		QuoteID:        uuid.NewSHA1(maturedSavingsNamespace, []byte("quote:"+account.Id+":"+day)).String(),
		IdempotencyKey: uuid.NewSHA1(maturedSavingsNamespace, []byte("key:"+account.Id+":"+day)).String(),
		Principal:      maturedSavingsPrincipal,
		TermMonths:     maturedSavingsTerm,
		AnnualRateBps:  rate,
		OpenedAt:       openedAt,
	})
	if err != nil {
		return fmt.Errorf("seed matured savings: %w", err)
	}
	logger.NewLogger(zerolog.InfoLevel).Info().
		Str("savings_no", m.Contract.SavingsNo).
		Time("opened_at", m.Contract.OpenedAt).
		Time("maturity_at", m.Contract.MaturityAt).
		Bool("created", created).
		Msg("matured savings ready")
	return nil
}

// maturedTodayOpening returns an opening time whose maturity is no later than
// now in Vietnam time (one hour earlier, or 00:00 near day start).
func maturedTodayOpening(now time.Time, termMonths int) (time.Time, error) {
	now = now.In(timeutil.VietnamLocation).Truncate(time.Second)
	target := now.Add(-time.Hour)
	if dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()); target.Before(dayStart) {
		target = dayStart
	}
	// Some calendar dates cannot be reached by adding this term from any valid
	// opening date (for example, February cannot produce a May 30 maturity).
	// Pick the latest reachable maturity at or before the target instead.
	for daysBack := 0; daysBack < 31; daysBack++ {
		candidateMaturity := target.AddDate(0, 0, -daysBack)
		openingMonth := time.Date(candidateMaturity.Year(), candidateMaturity.Month()-time.Month(termMonths), 1, 0, 0, 0, 0, candidateMaturity.Location())
		lastDay := openingMonth.AddDate(0, 1, -1).Day()
		for day := 1; day <= lastDay; day++ {
			openedAt := time.Date(openingMonth.Year(), openingMonth.Month(), day,
				candidateMaturity.Hour(), candidateMaturity.Minute(), candidateMaturity.Second(), candidateMaturity.Nanosecond(),
				candidateMaturity.Location())
			maturity, err := savings.MaturityForStoredContract(openedAt, termMonths)
			if err == nil && maturity.Equal(candidateMaturity) {
				return openedAt.UTC(), nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("no reachable %d-month maturity on or before %s", termMonths, target)
}
