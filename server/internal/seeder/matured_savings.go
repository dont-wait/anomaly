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

// seedMaturedSavings opens a savings contract that matured earlier today for
// the account whose CCCD is in SEED_MATURED_SAVINGS_CCCD. It does nothing when
// the variable is unset, so real identities never live in the repository.
// One contract is created per account per Vietnam calendar day.
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

// maturedTodayOpening returns an opening time whose maturity falls on now's
// Vietnam calendar day and no later than now (at least a minute earlier, or
// 00:00 when the day has just started).
func maturedTodayOpening(now time.Time, termMonths int) (time.Time, error) {
	now = now.In(timeutil.VietnamLocation).Truncate(time.Second)
	target := now.Add(-time.Hour)
	if dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()); target.Before(dayStart) {
		target = dayStart
	}
	// Months have different lengths, so search back from the naive guess for
	// the opening whose calendar-month maturity lands exactly on target.
	guess := target.AddDate(0, -termMonths, 0)
	for offset := -3; offset <= 3; offset++ {
		openedAt := guess.AddDate(0, 0, offset)
		maturity, err := savings.MaturityForStoredContract(openedAt, termMonths)
		if err == nil && maturity.Equal(target) {
			return openedAt.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("no %d-month opening matures at %s", termMonths, target)
}
