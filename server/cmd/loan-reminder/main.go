package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/dont-wait/anomaly/internal/application/loan/reminder"
	"github.com/dont-wait/anomaly/internal/composition"
	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/logger"
	"github.com/dont-wait/anomaly/internal/scheduler"
	"github.com/dont-wait/anomaly/internal/timeutil"
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

// notificationScanSlot permits morning and afternoon scans (09:00 and 16:00
// onwards, Vietnam local time). The hourly ticker retries failed scans, but
// successful scans are not repeated every hour. Restarts may revisit a slot;
// Notification's deterministic $setOnInsert handles cross-restart dedupe.
func notificationScanSlot(now time.Time) string {
	local := now.In(timeutil.VietnamLocation)
	hour := local.Hour()
	if hour < 9 || hour >= 21 {
		return ""
	}
	slot := "morning"
	if hour >= 16 {
		slot = "afternoon"
	}
	return local.Format("2006-01-02") + ":" + slot
}

// newReminderRunner is the production scheduler wiring. Tests call this exact
// function instead of duplicating the closure in main.
func newReminderRunner(log *zerolog.Logger, job reminder.Job, mode string, timeout time.Duration, opts ...scheduler.Option) (*scheduler.Scheduler, error) {
	var mu sync.Mutex
	lastSuccessfulSlot := ""
	return scheduler.New(log, []scheduler.Job{{
		Name:     "loan-repayment-reminder",
		Interval: interval,
		Timeout:  timeout,
		Run: func(scanCtx context.Context, now time.Time) error {
			if mode == "notification" {
				slot := notificationScanSlot(now)
				if slot == "" { // Never push to customers at midnight.
					log.Debug().Msg("loan reminder scan deferred outside notification hours")
					return nil
				}
				mu.Lock()
				alreadyScanned := slot == lastSuccessfulSlot
				mu.Unlock()
				if alreadyScanned {
					return nil
				}
			}

			stats, err := job.Run(scanCtx, now)
			if err != nil {
				// Do not report a failed scan as an empty successful scan.
				log.Error().Int("scanned", stats.Scanned).Int("matched", stats.Matched).
					Int("skipped", stats.Skipped).Int("failed", stats.Failed).
					Str("mode", mode).Msg("loan reminder scan failed")
				return err
			}
			if mode == "notification" {
				mu.Lock()
				lastSuccessfulSlot = notificationScanSlot(now)
				mu.Unlock()
			}
			log.Info().Int("scanned", stats.Scanned).Int("matched", stats.Matched).
				Int("skipped", stats.Skipped).Int("failed", stats.Failed).
				Str("mode", mode).Msg("loan reminder scan completed")
			return nil
		},
	}}, opts...)
}

func run(ctx context.Context, args []string, log *zerolog.Logger) error {
	if len(args) > 1 || (len(args) == 1 && args[0] != "--once") {
		return errors.New("usage: go run ./cmd/loan-reminder [--once]")
	}

	loader := domain.GetEnvLoader().Load(log)
	config := loader.LoadMongoConfig()
	client, err := mongo.NewMongoClient(ctx, config)
	if err != nil {
		return fmt.Errorf("connect loan reminder MongoDB: %w", err)
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
		store := mongo.NewNotificationRepository(client, config.MongoDBName)
		if err := store.CheckReady(ctx); err != nil {
			// Preserve the real reason (missing schema, auth, network, etc.) rather
			// than blaming migrations for every startup error.
			return fmt.Errorf("notification store readiness check failed: %w", err)
		}
		handler = reminder.NotificationHandler{
			Lookup:   mongo.NewLoanReminderRecipient(client, config.MongoDBName),
			Notifier: composition.NewNotifier(client, config.MongoDBName, loader.LoadNotificationConfig()),
		}
		mode = "notification"
	}

	job := reminder.Job{Repository: mongo.NewLoanReminderQuery(client, config.MongoDBName), Handler: handler}
	runner, err := newReminderRunner(log, job, mode, scanTimeout)
	if err != nil {
		return fmt.Errorf("configure loan reminder scheduler: %w", err)
	}
	if len(args) == 1 { // --once: failures produce nonzero exit after cleanup.
		return runner.RunOnce(ctx)
	}

	log.Info().Dur("interval", interval).Str("mode", mode).Msg("loan repayment reminder scheduler started")
	runner.Start(ctx)
	log.Info().Msg("loan repayment reminder scheduler stopped")
	return nil
}

// Keep signal.Stop and MongoDB.Disconnect on the normal return path. Only
// main() calls os.Exit, after all defers inside runProcess/run have executed.
func runProcess(args []string, log *zerolog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, args, log)
}

func main() {
	log := logger.NewLogger(zerolog.InfoLevel)
	if err := runProcess(os.Args[1:], log); err != nil {
		log.Error().Err(err).Msg("loan reminder worker failed")
		os.Exit(1)
	}
}
