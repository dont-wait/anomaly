package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dont-wait/anomaly/internal/application/loan/commands"
	"github.com/dont-wait/anomaly/internal/domain"
	loandomain "github.com/dont-wait/anomaly/internal/domain/loan"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	"github.com/dont-wait/anomaly/internal/infrastructure/loanadapter"
	"github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/logger"

	esdb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
	"github.com/rs/zerolog"
)

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	log := logger.NewLogger(zerolog.InfoLevel)
	loader := domain.GetEnvLoader().Load(log)
	conf := loader.LoadAllConfig()

	client, err := mongo.NewMongoClient(
		ctx,
		conf.MongoConfig,
	)
	if err != nil {
		log.Fatal().Err(err).Msg("loan Mongo connection")
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			log.Error().Err(err).Msg("disconnect MongoDB failed")
		}
	}()

	read := mongo.NewLoanReadModel(
		client,
		conf.MongoConfig.MongoDBName,
	)

	if err := read.EnsureIndexes(ctx); err != nil {
		log.Fatal().Err(err).Msg("loan indexes")
	}
	if err := read.EnsureFailureIndexes(ctx); err != nil {
		log.Fatal().Err(err).Msg("loan projection failure indexes")
	}

	es, err := eventstore.NewEventStoreClient(
		conf.EventStoreConfig,
	)
	if err != nil {
		log.Fatal().Err(err).
			Msg("loan EventStoreDB connection")
	}
	defer eventstore.Disconnect(es)

	repo := eventstore.NewLoanRepository(es)

	if len(os.Args) == 2 && os.Args[1] == "--backfill-loan-timeline" {
		count, err := backfillLoanTimeline(ctx, es, read)
		if err != nil {
			log.Fatal().Err(err).Msg("loan timeline backfill failed")
		}
		log.Info().Int("events", count).Msg("loan timeline backfill completed")
		return
	}

	// Chỉ cho phép reset Loan Read Model khi quản trị viên
	// chủ động yêu cầu và xác nhận đã sao lưu dữ liệu.
	if len(os.Args) == 2 &&
		os.Args[1] == "--reset-loan-projection" {

		if os.Getenv("LOAN_PROJECTION_RESET_CONFIRM") !=
			"I_HAVE_BACKED_UP_LOAN_READ_MODEL" {
			log.Fatal().Msg(
				"reset denied: backup confirmation is required",
			)
		}

		if err := read.ResetAfterInvalidCheckpoint(ctx); err != nil {
			log.Fatal().
				Err(err).
				Msg("reset loan projection failed")
		}

		log.Warn().Msg(
			"loan projection reset completed; restart projector to rebuild",
		)
		return
	}

	// Operator-only completion after replay reaches the EventStoreDB head.
	// Pause writes and stop the regular projector before using this command.
	if len(os.Args) == 2 && os.Args[1] == "--finalize-loan-recovery" {
		if os.Getenv("LOAN_PROJECTION_READY_CONFIRM") != "I_VERIFIED_LOAN_REPLAY" {
			log.Fatal().Msg("recovery finalization denied: explicit confirmation required")
		}
		rebuilding, err := read.LoanProjectionRebuilding(ctx)
		if err != nil {
			log.Fatal().Err(err).Msg("read loan recovery state")
		}
		if !rebuilding {
			log.Fatal().Msg("loan projection is not rebuilding")
		}
		checkpoint, found, err := read.LoadCheckpoint(ctx)
		if err != nil {
			log.Fatal().Err(err).Msg("read replay checkpoint")
		}
		if !found || checkpoint.EventID == "" {
			log.Fatal().Msg("no replay checkpoint to verify")
		}
		invalid, err := checkpointAheadOfStore(ctx, es, checkpoint)
		if err != nil {
			log.Fatal().Err(err).Msg("verify checkpoint")
		}
		if invalid {
			log.Fatal().Msg("replay checkpoint is invalid")
		}
		head, err := latestEventStorePosition(ctx, es)
		if err != nil {
			log.Fatal().Err(err).Msg("read EventStoreDB head")
		}
		if head.Commit != checkpoint.Commit || head.Prepare != checkpoint.Prepare || head.EventID != checkpoint.EventID {
			log.Fatal().Msg("replay has not reached current EventStoreDB head")
		}
		failures, err := read.HasOpenLoanProjectionFailures(ctx)
		if err != nil {
			log.Fatal().Err(err).Msg("check projection failures")
		}
		if failures {
			log.Fatal().Msg("unresolved loan projection failures")
		}
		if err := read.MarkLoanProjectionReady(ctx); err != nil {
			log.Fatal().Err(err).Msg("finalize loan recovery")
		}
		log.Info().Msg("loan read model verified and marked ready")
		return
	}

	// Backfill Read Model tra no cho cac khoan vay
	// da duoc projection tu truoc.
	// Khong xoa checkpoint hoac du lieu thanh toan.
	if len(os.Args) == 2 &&
		os.Args[1] == "--backfill-loan-repayment" {

		count, err := backfillLoanContracts(ctx, read)
		if err != nil {
			log.Fatal().
				Err(err).
				Msg("loan repayment backfill failed")
		}

		log.Info().
			Int("loans", count).
			Msg("loan repayment backfill completed")

		return
	}

	// Chế độ quản trị: thử replay lại một hồ sơ đã bị cách ly.
	if len(os.Args) > 1 {
		if len(os.Args) != 3 || os.Args[1] != "--retry-loan" {
			log.Fatal().Msg("usage: loan-projector --retry-loan <loan-id>")
		}

		loanID := strings.TrimSpace(os.Args[2])

		open, err := read.HasOpenProjectionFailure(ctx, loanID)
		if err != nil {
			log.Fatal().Err(err).Msg("check projection failure")
		}
		if !open {
			log.Info().Str("loan", loanID).
				Msg("no open projection failure")
			return
		}

		app, revision, err := repo.FindByIDWithRevision(ctx, loanID)
		if err != nil {
			log.Fatal().Err(err).Msg("loan replay retry failed")
		}
		if app == nil {
			log.Fatal().Str("loan", loanID).
				Msg("loan application not found")
		}

		if err := read.Save(ctx, app, revision); err != nil {
			log.Fatal().Err(err).Msg("save recovered loan failed")
		}

		if err := read.ResolveProjectionFailure(ctx, loanID); err != nil {
			log.Fatal().Err(err).Msg("resolve projection failure")
		}

		log.Info().Str("loan", loanID).
			Msg("loan projection recovered")
		return
	}

	// Banking Stream là nguồn dữ liệu giải ngân đã ghi thành công.
	bank := eventstore.NewBankRepository(
		es,
		mongo.NewTransferRepository(
			client,
			conf.MongoConfig.MongoDBName,
		),
	)

	// Dùng cùng EvidenceStore với Backend API.
	evidence := loanadapter.NewEvidenceStore(
		client,
		conf.MongoConfig.MongoDBName,
	)

	var legacyCutoff time.Time
	if value := strings.TrimSpace(os.Getenv("LOAN_OFFER_LEGACY_CREDIT_CUTOFF")); value != "" {
		legacyCutoff, err = time.Parse(time.RFC3339, value)
		if err != nil {
			log.Fatal().Err(err).Msg("invalid loan offer legacy credit cutoff")
		}
	}
	reconciler := &commands.ReconcileCreditedLoanCommandHandler{
		Repo:               repo,
		Evidence:           evidence,
		LegacyCreditCutoff: legacyCutoff,
	}

	// Phục hồi ngay khi khởi động, sau đó kiểm tra định kỳ.
	// Chạy riêng để không chặn Loan Projector.
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()

		for {
			if ctx.Err() != nil {
				return
			}

			if err := reconcileCreditsOnce(
				ctx,
				bank.ListCreditedLoans,
				reconciler.Handle,
			); err != nil && ctx.Err() == nil {
				log.Error().
					Err(err).
					Msg("loan disbursement reconciliation failed")
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	for ctx.Err() == nil {
		checkpoint, found, err := read.LoadCheckpoint(ctx)
		if err != nil {
			log.Error().Err(err).Msg("loan checkpoint")
			retry(ctx)
			continue
		}

		var from esdb.AllPosition = esdb.Start{}

		if found {
			if checkpoint.EventID == "" {
				log.Error().
					Uint64("commit", checkpoint.Commit).
					Uint64("prepare", checkpoint.Prepare).
					Msg("legacy loan checkpoint has no event_id; manual recovery required")
				return
			}

			invalid, err := checkpointAheadOfStore(
				ctx,
				es,
				checkpoint,
			)
			if err != nil {
				log.Error().
					Err(err).
					Msg("check loan checkpoint position failed")
				retry(ctx)
				continue
			}

			if invalid {
				log.Error().
					Uint64("commit", checkpoint.Commit).
					Uint64("prepare", checkpoint.Prepare).
					Str("event_id", checkpoint.EventID).
					Msg("loan checkpoint does not match EventStoreDB; manual recovery required")
				return
			}

		}

		if found {
			from = esdb.Position{
				Commit:  checkpoint.Commit,
				Prepare: checkpoint.Prepare,
			}
		}

		sub, err := es.SubscribeToAll(
			ctx,
			esdb.SubscribeToAllOptions{
				From: from,
			},
		)
		if err != nil {
			log.Error().Err(err).Msg("loan subscribe")
			retry(ctx)
			continue
		}

		for ctx.Err() == nil {
			msg := sub.Recv()

			if msg.SubscriptionDropped != nil {
				log.Error().
					Err(msg.SubscriptionDropped.Error).
					Msg("loan projection dropped")
				break
			}

			if msg.EventAppeared == nil {
				continue
			}

			recorded := msg.EventAppeared.Event

			if strings.HasPrefix(
				recorded.StreamID,
				"loan-application-",
			) {
				id := strings.TrimPrefix(
					recorded.StreamID,
					"loan-application-",
				)

				quarantined, err := read.HasOpenProjectionFailure(
					ctx,
					id,
				)
				if err != nil {
					log.Error().Err(err).
						Str("loan", id).
						Msg("check loan quarantine failed")
					break
				}

				if quarantined {
					// Hồ sơ đã có lỗi lưu trong MongoDB.
					// Bỏ qua riêng hồ sơ này, chờ quản trị viên xử lý.
					log.Warn().
						Str("loan", id).
						Msg("loan projection quarantined")
				} else {
					app, rev, replayErr := repo.FindByIDWithRevision(
						ctx,
						id,
					)

					// New event types must not be skipped or quarantined:
					// only a newer compatible reader can continue safely.
					if errors.Is(replayErr, eventstore.ErrLoanUnsupportedEvent) {
						log.Error().Err(replayErr).Str("loan", id).Msg("unsupported loan event; upgrade projector before continuing")
						break
					}
					if replayErr != nil &&
						!errors.Is(replayErr, eventstore.ErrLoanInvalidReplay) {
						log.Error().
							Err(replayErr).
							Str("loan", id).
							Msg("loan replay unavailable; retrying")
						break
					}

					if replayErr == nil && app == nil {
						log.Error().
							Str("loan", id).
							Msg("loan stream missing; retrying")
						break
					}

					if replayErr != nil || app == nil {
						if replayErr == nil {
							replayErr = fmt.Errorf("loan stream missing")
						}

						position := mongo.LoanCheckpoint{
							Commit:  recorded.Position.Commit,
							Prepare: recorded.Position.Prepare,
						}

						// Lưu lỗi thành công rồi mới cho phép
						// checkpoint tiến tới event tiếp theo.
						if err := read.RecordProjectionFailure(
							ctx,
							id,
							position,
							replayErr,
						); err != nil {
							log.Error().Err(err).
								Str("loan", id).
								Msg("persist loan projection failure failed")
							break
						}

						log.Error().
							Err(replayErr).
							Str("loan", id).
							Msg("loan quarantined; operator repair required")
					} else {
						if err := read.Save(ctx, app, rev); err != nil {
							log.Error().Err(err).
								Str("loan", id).
								Msg("loan projection write failed")
							break
						}
						if err := read.SaveLoanTimelineEvent(
							ctx,
							app,
							recorded.EventID.String(),
							recorded.EventType,
							recorded.EventNumber,
							recorded.CreatedDate,
							recorded.Data,
						); err != nil {
							log.Error().Err(err).
								Str("loan", id).
								Msg("loan timeline write failed")
							break
						}
					}
				}

			}

			err = read.SaveCheckpoint(
				ctx,
				mongo.LoanCheckpoint{
					Commit:  recorded.Position.Commit,
					Prepare: recorded.Position.Prepare,
					EventID: recorded.EventID.String(),
				},
			)
			if err != nil {
				log.Error().
					Err(err).
					Msg("loan checkpoint write failed")
				break
			}
		}

		_ = sub.Close()
		retry(ctx)
	}
}

func retry(ctx context.Context) {
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
	}
}

func reconcileCreditsOnce(
	ctx context.Context,
	list func(context.Context) ([]eventstore.CreditedLoan, error),
	finalize func(
		context.Context,
		commands.ReconcileCreditedLoanCommand,
	) error,
) error {
	credits, err := list(ctx)
	if err != nil {
		return fmt.Errorf("list credited loans: %w", err)
	}

	var failures []error

	for _, credit := range credits {
		if err := ctx.Err(); err != nil {
			return err
		}

		err := finalize(
			ctx,
			commands.ReconcileCreditedLoanCommand{
				LoanID:     credit.LoanID,
				AccountID:  credit.AccountID,
				Amount:     credit.Amount,
				CreditedAt: credit.CreditedAt,
			},
		)
		if err != nil {
			failures = append(
				failures,
				fmt.Errorf(
					"loan %s: %w",
					credit.LoanID,
					err,
				),
			)
		}
	}

	return errors.Join(failures...)
}

func checkpointAheadOfStore(
	ctx context.Context,
	client *esdb.Client,
	checkpoint mongo.LoanCheckpoint,
) (bool, error) {
	// Checkpoint cũ chưa lưu EventID.
	// Không thể xác minh nên cần rebuild.
	if checkpoint.EventID == "" {
		return true, nil
	}

	// Đọc ngược từ đúng vị trí checkpoint.
	stream, err := client.ReadAll(
		ctx,
		esdb.ReadAllOptions{
			Direction: esdb.Forwards,
			From: esdb.Position{
				Commit:  checkpoint.Commit,
				Prepare: checkpoint.Prepare,
			},
		},
		1,
	)
	if err != nil {
		return false, err
	}
	defer stream.Close()

	event, err := stream.Recv()
	if errors.Is(err, io.EOF) {
		return true, nil
	}
	if err != nil {
		return false, err
	}

	recorded := event.OriginalEvent()
	if recorded == nil {
		return false, fmt.Errorf(
			"cannot determine checkpoint event",
		)
	}

	// Sai vị trí hoặc sai EventID thì checkpoint
	// không còn khớp với EventStoreDB.
	invalid := recorded.Position.Commit != checkpoint.Commit ||
		recorded.Position.Prepare != checkpoint.Prepare ||
		recorded.EventID.String() != checkpoint.EventID

	return invalid, nil
}

// latestEventStorePosition reads the last event across all streams.
func latestEventStorePosition(ctx context.Context, client *esdb.Client) (mongo.LoanCheckpoint, error) {
	stream, err := client.ReadAll(ctx, esdb.ReadAllOptions{
		Direction: esdb.Backwards,
		From:      esdb.End{},
	}, 1)
	if err != nil {
		return mongo.LoanCheckpoint{}, err
	}
	defer stream.Close()
	event, err := stream.Recv()
	if err != nil {
		return mongo.LoanCheckpoint{}, err
	}
	recorded := event.OriginalEvent()
	if recorded == nil {
		return mongo.LoanCheckpoint{}, errors.New("EventStoreDB head has no event")
	}
	return mongo.LoanCheckpoint{
		Commit:  recorded.Position.Commit,
		Prepare: recorded.Position.Prepare,
		EventID: recorded.EventID.String(),
	}, nil
}

// backfillLoanTimeline rebuilds the account-scoped loan timeline without
// touching the loan projector checkpoint or emitting new business events.
// ReadAll positions are inclusive, so replaying a page boundary is safe
// because timeline writes are idempotent by EventID.
func backfillLoanTimeline(ctx context.Context, client *esdb.Client, read *mongo.LoanReadModel) (int, error) {
	const pageSize = 4096
	apps := make(map[string]*loandomain.Application)
	var afterTime time.Time
	var afterID string
	for {
		items, hasMore, err := read.ListAllPage(ctx, 100, afterTime, afterID)
		if err != nil {
			return 0, err
		}
		for _, app := range items {
			if app != nil {
				apps[app.Id] = app
			}
		}
		if !hasMore || len(items) == 0 {
			break
		}
		last := items[len(items)-1]
		afterTime, afterID = last.CreatedAt, last.Id
	}

	count := 0
	opts := esdb.ReadAllOptions{}
	for {
		stream, err := client.ReadAll(ctx, opts, pageSize)
		if err != nil {
			return count, err
		}
		eventsInPage := 0
		var lastPosition esdb.Position
		for {
			event, recvErr := stream.Recv()
			if errors.Is(recvErr, io.EOF) {
				break
			}
			if recvErr != nil {
				stream.Close()
				return count, recvErr
			}
			eventsInPage++
			lastPosition = event.Event.Position
			if !strings.HasPrefix(event.Event.StreamID, "loan-application-") {
				continue
			}
			loanID := strings.TrimPrefix(event.Event.StreamID, "loan-application-")
			app := apps[loanID]
			if app == nil {
				continue
			}
			if err := read.SaveLoanTimelineEvent(
				ctx,
				app,
				event.Event.EventID.String(),
				event.Event.EventType,
				event.Event.EventNumber,
				event.Event.CreatedDate,
				event.Event.Data,
			); err != nil {
				stream.Close()
				return count, err
			}
			count++
		}
		stream.Close()
		if eventsInPage < pageSize {
			break
		}
		opts = esdb.ReadAllOptions{From: lastPosition}
	}
	return count, nil
}

func backfillLoanContracts(
	ctx context.Context,
	read *mongo.LoanReadModel,
) (int, error) {
	const pageSize = 100

	var afterTime time.Time
	var afterID string
	count := 0

	for {
		items, hasMore, err := read.ListAllPage(
			ctx,
			pageSize,
			afterTime,
			afterID,
		)
		if err != nil {
			return count, err
		}

		for _, app := range items {
			if app == nil {
				return count, fmt.Errorf(
					"invalid loan application read model",
				)
			}

			if err := read.SaveLoanContract(
				ctx,
				app,
			); err != nil {
				return count, fmt.Errorf(
					"backfill application %s: %w",
					app.Id,
					err,
				)
			}

			if app.Status == "disbursed" {
				count++
			}
		}

		if !hasMore {
			break
		}

		if len(items) == 0 {
			return count, fmt.Errorf(
				"loan backfill pagination did not advance",
			)
		}

		last := items[len(items)-1]
		afterTime = last.CreatedAt
		afterID = last.Id
	}

	return count, nil
}
