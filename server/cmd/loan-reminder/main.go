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
	"github.com/rs/zerolog"
)

const (
	interval    = time.Hour
	scanTimeout = 2 * time.Minute
)

// runScan bounds each MongoDB scan; a slow query cannot stall the scheduler
// indefinitely. The parent context still handles shutdown signals.
func runScan(ctx context.Context, job reminder.Job, now time.Time, timeout time.Duration) (reminder.Stats, error) {
	scanCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return job.Run(scanCtx, now)
}

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

	run := func() error {
		stats, err := runScan(ctx, job, time.Now(), scanTimeout)
		if err != nil {
			return err
		}
		log.Info().Int("scanned", stats.Scanned).
			Int("matched", stats.Matched).
			Int("skipped", stats.Skipped).
			Int("failed", stats.Failed).
			Msg("loan reminder scan completed (dry run; sent=0)")
		return nil
	}

	if err := run(); err != nil {
		if len(os.Args) == 2 { // --once must report a failed scan to the caller.
			log.Fatal().Err(err).Msg("loan reminder one-shot scan failed")
		}
		log.Error().Err(err).Msg("loan reminder scan failed")
	}
	if len(os.Args) == 2 { // --once
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	log.Info().Dur("interval", interval).Msg("loan repayment reminder scheduler started (dry run)")
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("loan repayment reminder scheduler stopped")
			return
		case <-ticker.C:
			if err := run(); err != nil {
				log.Error().Err(err).Msg("loan reminder scan failed")
			}
		}
	}
}
