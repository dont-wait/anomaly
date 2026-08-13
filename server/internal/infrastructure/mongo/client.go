package mongo

import (
	"context"
	"time"

	"github.com/rs/zerolog"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func NewClient(ctx context.Context, logger zerolog.Logger, uri string) (*mongodrv.Client, error) {
	client, err := mongodrv.Connect(options.Client().ApplyURI(uri))
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
