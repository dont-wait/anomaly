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
	kurrentdb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
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

	mongoRepo := mongo.NewAccountRepository(mongoClient, config.MongoConfig.MongoDBName)
	if err := mongoRepo.EnsureIndexes(ctx); err != nil {
		log.Fatal().Err(err).Msg("ensure mongo indexes failed")
	}
	checkpointRepo := mongo.NewCheckpointRepository(mongoClient, config.MongoConfig.MongoDBName)

	esClient, err := eventstore.NewEventStoreClient(config.EventStoreConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("connect event store failed")
	}
	defer eventstore.Disconnect(esClient)

	esRepo := eventstore.NewAccountRepository(esClient)

	log.Info().Msg("projection worker started")
	runWithReconnect(ctx, esClient, mongoRepo, esRepo, checkpointRepo, log)
	log.Info().Msg("projection worker stopped")
}

// runWithReconnect subscribe vào $all và tự động reconnect (kèm backoff
// tăng dần, tối đa maxBackoff) mỗi khi subscription bị drop. Mỗi lần
// (re)subscribe, resume từ checkpoint đã lưu gần nhất trong Mongo thay
// vì luôn replay lại từ đầu hoặc bỏ sót event.
func runWithReconnect(
	ctx context.Context,
	esClient *kurrentdb.Client,
	mongoRepo *mongo.AccountRepository,
	esRepo *eventstore.AccountRepository,
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

		sub, err := esClient.SubscribeToAll(
			ctx,
			kurrentdb.SubscribeToAllOptions{From: from})
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

		dropped := consume(ctx, sub, mongoRepo, esRepo, checkpointRepo, log)
		sub.Close()

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
func resolveFrom(ctx context.Context, checkpointRepo *mongo.CheckpointRepository, log *zerolog.Logger) (kurrentdb.AllPosition, error) {
	commit, prepare, found, err := checkpointRepo.Load(ctx)
	if err != nil {
		return nil, err
	}
	if !found {
		log.Info().Msg("no checkpoint found, subscribing from start")
		return kurrentdb.Start{}, nil
	}
	log.Info().Uint64("commit", commit).Uint64("prepare", prepare).Msg("resuming from checkpoint")
	return kurrentdb.Position{Commit: commit, Prepare: prepare}, nil
}

// consume đọc event tới khi subscription bị drop hoặc ctx bị huỷ.
// Trả về true nếu dừng do subscription drop (cần reconnect), false nếu
// dừng do ctx bị huỷ (graceful shutdown, không cần reconnect).
func consume(
	ctx context.Context,
	sub *kurrentdb.Subscription,
	mongoRepo *mongo.AccountRepository,
	esRepo *eventstore.AccountRepository,
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

		if err := handleEvent(ctx, mongoRepo, esRepo, log, recorded.EventType, recorded.StreamID); err != nil {
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
	mongoRepo *mongo.AccountRepository,
	esRepo *eventstore.AccountRepository,
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

	acc, err := esRepo.FindByID(ctx, accountID)
	if err != nil {
		return fmt.Errorf("replay account %s failed: %w", accountID, err)
	}
	if acc == nil {
		log.Warn().Str("streamId", streamID).Msg("account not found after replay")
		return nil
	}

	if err := mongoRepo.Save(ctx, acc); err != nil {
		// Mongo từ chối vì vi phạm unique index (trùng email/username với
		// account khác đã có). Đây là xung đột dữ liệu — KHÔNG retry, vẫn
		// lưu checkpoint để không kẹt subscription.
		if mongo.IsDuplicateKeyError(err) {
			log.Warn().Err(err).Str("accountId", acc.Id).Msg("skip projection: duplicate key (email/username collision)")
			return nil
		}
		return fmt.Errorf("upsert account %s into mongo failed: %w", acc.Id, err)
	}

	log.Info().Str("accountId", acc.Id).Int64("amount", acc.Amount).Bool("isVerify", acc.IsVerify).Msg("projected account into mongo")
	return nil
}
