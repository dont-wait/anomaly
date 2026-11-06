// Package scheduler runs recurring background jobs without embedding ticker,
// per-run deadlines, or shutdown handling in each business worker.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// Job has its own interval and timeout. Run must respect context cancellation.
// A timeout cannot forcibly interrupt implementations that ignore their context.
type Job struct {
	Name     string
	Interval time.Duration
	Timeout  time.Duration
	Run      func(ctx context.Context, now time.Time) error
}

type Option func(*Scheduler) error

// WithNow replaces the clock for deterministic tests.
func WithNow(now func() time.Time) Option {
	return func(s *Scheduler) error {
		if now == nil {
			return errors.New("scheduler clock must not be nil")
		}
		s.now = now
		return nil
	}
}

type Scheduler struct {
	log  *zerolog.Logger
	jobs []Job
	now  func() time.Time
}

// New validates and copies jobs, so callers cannot mutate their configuration.
func New(log *zerolog.Logger, jobs []Job, opts ...Option) (*Scheduler, error) {
	if log == nil {
		return nil, errors.New("scheduler logger is required")
	}
	if len(jobs) == 0 {
		return nil, errors.New("scheduler requires at least one job")
	}

	s := &Scheduler{log: log, jobs: append([]Job(nil), jobs...), now: time.Now}
	seen := make(map[string]struct{}, len(jobs))
	for _, job := range s.jobs {
		if strings.TrimSpace(job.Name) == "" || job.Name != strings.TrimSpace(job.Name) {
			return nil, errors.New("scheduler job name must not be empty or padded")
		}
		if _, exists := seen[job.Name]; exists {
			return nil, fmt.Errorf("duplicate scheduler job name %q", job.Name)
		}
		seen[job.Name] = struct{}{}
		if job.Interval <= 0 || job.Timeout <= 0 || job.Run == nil {
			return nil, fmt.Errorf("scheduler job %q needs a positive interval, timeout, and Run function", job.Name)
		}
	}
	for _, option := range opts {
		if option == nil {
			return nil, errors.New("nil scheduler option")
		}
		if err := option(s); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Scheduler) runJob(ctx context.Context, job Job) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("scheduler job %q: %w", job.Name, err)
	}
	runCtx, cancel := context.WithTimeout(ctx, job.Timeout)
	defer cancel()

	err := job.Run(runCtx, s.now())
	if err != nil {
		return fmt.Errorf("scheduler job %q: %w", job.Name, err)
	}
	if err := runCtx.Err(); err != nil {
		return fmt.Errorf("scheduler job %q: %w", job.Name, err)
	}
	return nil
}

// RunOnce runs every registered job once, continuing after individual failures
// and joining their errors so a one-shot invocation can report failure.
func (s *Scheduler) RunOnce(ctx context.Context) error {
	var failures []error
	for _, job := range s.jobs {
		if err := s.runJob(ctx, job); err != nil {
			failures = append(failures, err)
		}
		if ctx.Err() != nil {
			break
		}
	}
	return errors.Join(failures...)
}

// Start runs each job immediately, then at its own interval until ctx is
// cancelled. A slow job cannot block the ticker loop of another job.
func (s *Scheduler) Start(ctx context.Context) {
	var wg sync.WaitGroup
	for _, job := range s.jobs {
		wg.Add(1)
		go func(job Job) {
			defer wg.Done()
			ticker := time.NewTicker(job.Interval)
			defer ticker.Stop()
			for {
				if ctx.Err() != nil {
					return
				}
				if err := s.runJob(ctx, job); err != nil && ctx.Err() == nil {
					// A reminder handler error exposes only its type in Error().
					// Business jobs must not return unredacted customer data.
					s.log.Error().Str("job", job.Name).Err(err).Msg("scheduled job failed")
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}(job)
	}
	wg.Wait()
}
