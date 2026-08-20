package main

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	"github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/logger"
	kurrentdb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
)

func main() {
	ctx := context.Background()
	log := logger.NewLogger(zerolog.InfoLevel)

	loader := domain.GetEnvLoader().Load(log)
	mongoConf := loader.LoadMongoConfig()
	esConf := loader.LoadEventStoreConfig()

	mongoClient, err := mongo.NewMongoClient(ctx, mongoConf)
	if err != nil {
		log.Fatal().Err(err).Msg("connect mongo failed")
	}
	defer func() {
		_ = mongoClient.Disconnect(ctx)
	}()
	mongoRepo := mongo.NewAccountRepository(mongoClient, mongoConf.MongoDBName)

	esClient, err := eventstore.NewEventStoreClient(esConf)
	if err != nil {
		log.Fatal().Err(err).Msg("connect event store failed")
	}
	defer eventstore.Disconnect(esClient)

	log.Info().Msg("projection worker started, subscribing to $all...")

	sub, err := esClient.SubscribeToAll(ctx, kurrentdb.SubscribeToAllOptions{})
	if err != nil {
		log.Fatal().Err(err).Msg("subscribe to $all failed")
	}
	defer sub.Close()

	for {
		event := sub.Recv()

		if event.SubscriptionDropped != nil {
			log.Error().Err(event.SubscriptionDropped.Error).Msg("subscription dropped")
			break
		}

		if event.EventAppeared == nil {
			continue // CaughtUp signal hoặc tín hiệu khác, bỏ qua
		}

		recorded := event.EventAppeared.Event
		handleEvent(ctx, mongoRepo, esClient, log, recorded.EventType, recorded.StreamID)
	}
}

// handleEvent nhận biết stream nào vừa có event mới, rồi đọc lại (replay)
// TOÀN BỘ trạng thái mới nhất của account đó từ EventStore, upsert vào Mongo.
func handleEvent(
	ctx context.Context,
	mongoRepo *mongo.AccountRepository,
	esClient *kurrentdb.Client,
	log *zerolog.Logger,
	eventType string,
	streamID string,
) {
	// chỉ quan tâm event của account, bỏ qua event hệ thống ($...) hoặc stream khác
	if eventType != "AccountCreated" && eventType != "AccountWithdrew" {
		return
	}

	esRepo := eventstore.NewAccountRepository(esClient)
	accountID := streamID[len("account-"):] // "account-TK001" -> "TK001"

	acc, err := esRepo.FindByID(ctx, accountID)
	if err != nil || acc == nil {
		log.Warn().Err(err).Str("streamId", streamID).Msg("failed to replay account after event")
		return
	}

	if err := mongoRepo.Save(ctx, acc); err != nil {
		log.Warn().Err(err).Str("accountId", acc.Id).Msg("failed to upsert account into mongo read model")
		return
	}

	log.Info().Str("accountId", acc.Id).Int64("amount", acc.Amount).Msg("projected account into mongo")
}
