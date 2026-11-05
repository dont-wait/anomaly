package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dont-wait/anomaly/internal/application/loan/reminder"
	"github.com/dont-wait/anomaly/internal/composition"
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

	notificationsEnabled := loader.LoadEnvBool("LOAN_REMINDER_NOTIFICATIONS_ENABLED", false)
	mode := "dry-run"
	var handler reminder.Handler = reminder.HandlerFunc(func(_ context.Context, item reminder.Candidate) error {
		logDryRunCandidate(log, item)
		return nil
	})
	if notificationsEnabled {
		// Fail closed when migration 20 has not created notifications.
		store := mongo.NewNotificationRepository(client, config.MongoDBName)
		if err := store.EnsureIndexes(ctx); err != nil {
			log.Fatal().Msg("notification store is not ready; apply the notification migration first")
		}
		handler = reminder.NotificationHandler{
			Lookup:   mongo.NewLoanReminderRecipient(client, config.MongoDBName),
			Notifier: composition.NewNotifier(client, config.MongoDBName, loader.LoadNotificationConfig()),
		}
		mode = "notification"
	}
	job := reminder.Job{
		Repository: mongo.NewLoanReminderQuery(client, config.MongoDBName),
		Handler:    handler,
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
				Str("mode", mode).
				Msg("loan reminder scan completed")
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

	log.Info().Dur("interval", interval).Str("mode", mode).Msg("loan repayment reminder scheduler started")
	runner.Start(ctx)
	log.Info().Msg("loan repayment reminder scheduler stopped")
}
