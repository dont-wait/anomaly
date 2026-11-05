package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dont-wait/anomaly/internal/application/loan/reminder"
	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/logger"
	"github.com/dont-wait/anomaly/internal/scheduler"
	"github.com/rs/zerolog"
)

const (
	interval    = time.Hour
	scanTimeout = 2 * time.Minute
)

// Candidate identifiers and financial details must never enter routine logs.
func logDryRunCandidate(log *zerolog.Logger, item reminder.Candidate) {
	log.Info().Str("type", string(item.Type)).
		Msg("loan repayment reminder candidate (dry run; NOT sent)")
}

func main() {
	log := logger.NewLogger(zerolog.InfoLevel)
	if len(os.Args) > 2 || (len(os.Args) == 2 && os.Args[1] != "--once") {
		log.Fatal().Msg("usage: go run ./cmd/loan-reminder [--once]")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	loader := domain.GetEnvLoader().Load(log)
	config := loader.LoadMongoConfig()
	client, err := mongo.NewMongoClient(ctx, config)
	if err != nil {
		log.Fatal().Err(err).Msg("connect loan reminder MongoDB")
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := client.Disconnect(cleanupCtx); err != nil {
			log.Error().Err(err).Msg("disconnect loan reminder MongoDB")
		}
	}()

	job := reminder.Job{
		Repository: mongo.NewLoanReminderQuery(client, config.MongoDBName),
		// Phase 1: safe, read-only dry-run. Never send email or reserve keys.
		Handler: reminder.HandlerFunc(func(_ context.Context, item reminder.Candidate) error {
			logDryRunCandidate(log, item)
			return nil
		}),
	}

	runner, err := scheduler.New(log, []scheduler.Job{{
		Name:     "loan-repayment-reminder",
		Interval: interval,
		Timeout:  scanTimeout,
		Run: func(scanCtx context.Context, now time.Time) error {
			stats, err := job.Run(scanCtx, now)
			log.Info().Int("scanned", stats.Scanned).
				Int("matched", stats.Matched).
				Int("skipped", stats.Skipped).
				Int("failed", stats.Failed).
				Msg("loan reminder scan completed (dry run; sent=0)")
			return err
		},
	}})
	if err != nil {
		log.Fatal().Err(err).Msg("configure loan reminder scheduler")
	}

	if len(os.Args) == 2 { // --once: failures must produce a nonzero exit code.
		if err := runner.RunOnce(ctx); err != nil {
			log.Fatal().Err(err).Msg("loan reminder one-shot scan failed")
		}
		return
	}

	log.Info().Dur("interval", interval).Msg("loan repayment reminder scheduler started (dry run)")
	runner.Start(ctx)
	log.Info().Msg("loan repayment reminder scheduler stopped")
}
