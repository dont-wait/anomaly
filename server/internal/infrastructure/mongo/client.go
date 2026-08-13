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
		client.Disconnect(ctx)
		return nil, err
	}

	logger.Info().Msg("connected to mongodb")

	return client, nil
}
