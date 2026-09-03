package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	"github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/logger"
	eventstoredb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
)

const (
	initialBackoff = 2 * time.Second
	maxBackoff     = 30 * time.Second
)

func main() {
	// Bắt SIGINT/SIGTERM để dừng worker sạch sẽ (Ctrl+C, docker stop...)
	// thay vì để nó bị kill giữa chừng.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log := logger.NewLogger(zerolog.InfoLevel)

	loader := domain.GetEnvLoader().Load(log)
	config := loader.LoadAllConfig()

	mongoClient, err := mongo.NewMongoClient(ctx, config.MongoConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("connect mongo failed")
	}
	defer func() {
		_ = mongoClient.Disconnect(context.Background())
	}()

	accountRepo := mongo.NewAccountRepository(mongoClient, config.MongoConfig.MongoDBName)
	if err := accountRepo.EnsureIndexes(ctx); err != nil {
		log.Fatal().Err(err).Msg("ensure mongo indexes failed")
	}
	customerRepo := mongo.NewCustomerRepository(mongoClient, config.MongoConfig.MongoDBName)
	if err := customerRepo.EnsureIndexes(ctx); err != nil {
		log.Fatal().Err(err).Msg("ensure customer indexes failed")
	}
	kycRepo := mongo.NewKYCSessionRepository(mongoClient, config.MongoConfig.MongoDBName)
	if err := kycRepo.EnsureIndexes(ctx); err != nil {
		log.Fatal().Err(err).Msg("ensure KYC session indexes failed")
	}
	checkpointRepo := mongo.NewCheckpointRepository(mongoClient, config.MongoConfig.MongoDBName)

	eventStoreClient, err := eventstore.NewEventStoreClient(config.EventStoreConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("connect event store failed")
	}
	defer eventstore.Disconnect(eventStoreClient)

	eventStoreRepo := eventstore.NewAccountRepository(eventStoreClient)

	log.Info().Msg("projection worker started")
	runWithReconnect(ctx, eventStoreClient, accountRepo, customerRepo, kycRepo, eventStoreRepo, checkpointRepo, log)
	log.Info().Msg("projection worker stopped")
}

// runWithReconnect subscribe vào $all và tự động reconnect (kèm backoff
// tăng dần, tối đa maxBackoff) mỗi khi subscription bị drop. Mỗi lần
// (re)subscribe, resume từ checkpoint đã lưu gần nhất trong Mongo thay
// vì luôn replay lại từ đầu hoặc bỏ sót event.
func runWithReconnect(
	ctx context.Context,
	eventStoreClient *eventstoredb.Client,
	accountRepo *mongo.AccountRepository,
	customerRepo *mongo.CustomerRepository,
	kycRepo *mongo.KYCSessionRepository,
	eventStoreRepo *eventstore.AccountRepository,
	checkpointRepo *mongo.CheckpointRepository,
	log *zerolog.Logger,
) {
	backoff := initialBackoff

	for {
		if ctx.Err() != nil {
			return
		}

		from, err := resolveFrom(ctx, checkpointRepo, log)
		if err != nil {
			log.Error().Err(err).Msg("load checkpoint failed, retrying")
			if !sleepOrDone(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}

		sub, err := eventStoreClient.SubscribeToAll(
			ctx,
			eventstoredb.SubscribeToAllOptions{From: from})
		if err != nil {
			log.Error().Err(err).Msg("subscribe to $all failed, retrying")
			if !sleepOrDone(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}

		log.Info().Msg("subscribed to $all")
		backoff = initialBackoff // reset backoff mỗi khi subscribe thành công

		dropped := consume(ctx, sub, accountRepo, customerRepo, kycRepo, eventStoreRepo, checkpointRepo, log)
		if err := sub.Close(); err != nil {
			log.Warn().Err(err).Msg("close subscription failed")
		}

		if !dropped {
			// vòng lặp dừng vì ctx bị cancel (graceful shutdown),
			// không phải vì subscription bị drop -> thoát hẳn.
			return
		}

		if !sleepOrDone(ctx, backoff) {
			return
		}
		backoff = nextBackoff(backoff)
	}
}

// resolveFrom quyết định subscribe $all từ đâu: từ checkpoint đã lưu
// (resume), hoặc từ đầu nếu chưa từng có checkpoint (lần chạy đầu).
func resolveFrom(ctx context.Context, checkpointRepo *mongo.CheckpointRepository, log *zerolog.Logger) (eventstoredb.AllPosition, error) {
	commit, prepare, found, err := checkpointRepo.Load(ctx)
	if err != nil {
		return nil, err
	}
	if !found {
		log.Info().Msg("no checkpoint found, subscribing from start")
		return eventstoredb.Start{}, nil
	}
	log.Info().Uint64("commit", commit).Uint64("prepare", prepare).Msg("resuming from checkpoint")
	return eventstoredb.Position{Commit: commit, Prepare: prepare}, nil
}

// consume đọc event tới khi subscription bị drop hoặc ctx bị huỷ.
// Trả về true nếu dừng do subscription drop (cần reconnect), false nếu
// dừng do ctx bị huỷ (graceful shutdown, không cần reconnect).
func consume(
	ctx context.Context,
	sub *eventstoredb.Subscription,
	accountRepo *mongo.AccountRepository,
	customerRepo *mongo.CustomerRepository,
	kycRepo *mongo.KYCSessionRepository,
	eventStoreRepo *eventstore.AccountRepository,
	checkpointRepo *mongo.CheckpointRepository,
	log *zerolog.Logger,
) bool {
	for {
		if ctx.Err() != nil {
			return false
		}

		event := sub.Recv()

		if event.SubscriptionDropped != nil {
			log.Error().Err(event.SubscriptionDropped.Error).Msg("subscription dropped, will reconnect")
			return true
		}
		if event.EventAppeared == nil {
			log.Debug().Msg("received non-event signal (e.g. CaughtUp), skipping") // nhận được tín hiệu không phải event (ví dụ CaughtUp), đang bỏ qua
			continue                                                               // CaughtUp signal hoặc tín hiệu khác, bỏ qua
		}

		recorded := event.EventAppeared.Event

		if err := handleEvent(
			ctx,
			accountRepo,
			customerRepo,
			kycRepo,
			eventStoreRepo,
			log,
			recorded.EventType,
			recorded.StreamID,
		); err != nil {
			// Xử lý thất bại -> KHÔNG lưu checkpoint, để lần reconnect/restart
			// sau tự động đọc lại đúng event này (retry tự nhiên nhờ resume
			// từ checkpoint cũ). Log lỗi rồi dừng hẳn vòng đọc hiện tại, buộc
			// runWithReconnect backoff + subscribe lại từ checkpoint cũ.
			log.Error().Err(err).Str("streamId", recorded.StreamID).Msg("handle event failed, will retry from last checkpoint")
			return true
		}

		// Chỉ lưu checkpoint SAU KHI xử lý thành công, để đảm bảo event lỗi
		// luôn được thử lại ở lần chạy tiếp theo thay vì bị bỏ sót.
		if err := checkpointRepo.Save(ctx, recorded.Position.Commit, recorded.Position.Prepare); err != nil {
			log.Warn().Err(err).Msg("failed to save checkpoint")
		}
	}
}

func sleepOrDone(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

func nextBackoff(d time.Duration) time.Duration {
	next := d * 2
	if next > maxBackoff {
		return maxBackoff
	}
	return next
}

// handleEvent nhận biết stream nào vừa có event mới, rồi đọc lại (replay)
// TOÀN BỘ trạng thái mới nhất của account đó từ EventStore, upsert vào Mongo.
func handleEvent(
	ctx context.Context,
	accountRepo *mongo.AccountRepository,
	customerRepo *mongo.CustomerRepository,
	kycRepo *mongo.KYCSessionRepository,
	eventStoreRepo *eventstore.AccountRepository,
	log *zerolog.Logger,
	eventType string,
	streamID string,
) error {
	// event không liên quan tới account -> không có gì để làm,
	// coi như "xử lý thành công" (không phải lỗi) để checkpoint vẫn tiến lên
	if eventType != eventstore.EventAccountCreated &&
		eventType != eventstore.EventAccountVerified &&
		eventType != eventstore.EventAccountWithdraw {
		return nil
	}

	if !strings.HasPrefix(streamID, "account-") {
		log.Warn().Str("streamId", streamID).Msg("unexpected stream id format")
		return nil // dữ liệu lạ, không phải lỗi tạm thời -> không cần retry
	}
	accountID := strings.TrimPrefix(streamID, "account-")

	acc, err := eventStoreRepo.FindByID(ctx, accountID)
	if err != nil {
		return fmt.Errorf("replay account %s failed: %w", accountID, err)
	}
	if acc == nil {
		log.Warn().Str("streamId", streamID).Msg("account not found after replay")
		return nil
	}

	for _, session := range acc.KYCSessions {
		if err := kycRepo.Save(ctx, session); err != nil {
			return fmt.Errorf("project KYC session %s failed: %w", session.Id, err)
		}
	}
	if acc.Customer == nil {
		return fmt.Errorf("project account %s failed: customer is missing", acc.Id)
	}
	if err := customerRepo.Save(ctx, acc.Customer); err != nil {
		return fmt.Errorf("project customer %s failed: %w", acc.Customer.Id, err)
	}
	if err := accountRepo.Save(ctx, acc); err != nil {
		// Mongo từ chối vì vi phạm unique index (trùng email/username với
		// account khác đã có). Giữ event chưa checkpoint để worker retry,
		// tránh làm mất account khỏi projection mà không có record xử lý lại.
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("project account %s failed: duplicate key (email/username collision): %w", acc.Id, err)
		}
		return fmt.Errorf("upsert account %s into mongo failed: %w", acc.Id, err)
	}

	log.Info().
		Str("accountId", acc.Id).
		Str("customerId", acc.CustomerId).
		Int64("amount", acc.Balance.Current).
		Bool("isVerify", acc.IsVerified()).
		Msg("projected account into mongo")
	return nil
}
