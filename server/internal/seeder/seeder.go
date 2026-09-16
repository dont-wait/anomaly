package seeder

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/application/account/commands"
	"github.com/dont-wait/anomaly/internal/logger"
	"github.com/rs/zerolog"
)

type Repository interface {
	commands.AccountCreator
	commands.AccountRepository
}

// Dependencies holds shared repositories used by seeders.
type Dependencies struct {
	Accounts Repository
}

type seedFunc func(context.Context, Dependencies) error
type registry map[string]seedFunc

var seeders = registry{}

// register is called by each seeder's init function. Names define execution order.
func (r registry) register(name string, run seedFunc) {
	if strings.TrimSpace(name) == "" || run == nil {
		panic("seeder requires a name and run function")
	}
	if _, exists := r[name]; exists {
		panic("duplicate seeder: " + name)
	}
	r[name] = run
}

func (r registry) run(ctx context.Context, deps Dependencies) error {
	log := logger.NewLogger(zerolog.InfoLevel)
	names := make([]string, 0, len(r))
	for name := range r {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		started := time.Now()
		log.Info().Str("seeder", name).Msg("seeder started")
		if err := r[name](ctx, deps); err != nil {
			return fmt.Errorf("seeder %s: %w", name, err)
		}
		log.Info().Str("seeder", name).Dur("duration", time.Since(started)).Msg("seeder completed")
	}
	return nil
}

// Run executes all seeders registered by files in this package, stopping on failure.
func Run(ctx context.Context, deps Dependencies) error {
	return seeders.run(ctx, deps)
}

func ValidateEnvironment(environment, enabled string) error {
	environment = strings.ToLower(strings.TrimSpace(environment))
	if environment != "development" && environment != "dev" && environment != "local" {
		return fmt.Errorf("APP_ENV must be development, dev, or local; got %q", environment)
	}
	if !strings.EqualFold(strings.TrimSpace(enabled), "true") {
		return errors.New("SEED_DEMO_ENABLED=true is required")
	}
	return nil
}
