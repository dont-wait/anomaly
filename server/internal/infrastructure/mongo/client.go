package mongo

import (
	"context"
	"time"

	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/logger"
	"github.com/rs/zerolog"
	"go.mongodb.org/mongo-driver/v2/mongo"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func NewMongoClient(ctx context.Context, mongoConf *domain.MongoConfig) (*mongo.Client, error) {
	logger := logger.NewLogger(zerolog.InfoLevel)
	opts := options.ClientOptions{}
	client, err := mongodrv.Connect(opts.ApplyURI(mongoConf.MongoURI))
	if err != nil {
		return nil, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := client.Ping(pingCtx, nil); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if dcErr := client.Disconnect(cleanupCtx); dcErr != nil {
			logger.Warn().Err(dcErr).Msg("failed to disconnect mongo client after ping failure")
		}
		return nil, err
	}

	logger.Info().Msg("connected to mongodb")

	return client, nil
}

func Disconnect(client *mongo.Client, ctx context.Context, cancel context.CancelFunc) {
	log := logger.NewLogger(zerolog.InfoLevel)
	defer cancel()
	defer func() {
		if err := client.Disconnect(ctx); err != nil {
			log.Err(err).Msg("error when disconnecting from MongoDB")
			panic(err)
		}
	}()
}
