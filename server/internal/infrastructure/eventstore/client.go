package eventstore

import (
	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/logger"
	"github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
	"github.com/rs/zerolog"
)

// tạo kết nối tới KurrentDB (event store).
func NewEventStoreClient(conf *domain.EventStoreConfig) (*kurrentdb.Client, error) {
	log := logger.NewLogger(zerolog.InfoLevel)

	settings, err := kurrentdb.ParseConnectionString(conf.EventStoreConnString)
	if err != nil {
		return nil, err
	}
	client, err := kurrentdb.NewClient(settings)
	if err != nil {
		return nil, err
	}
	log.Info().Msg("Connected to event store")
	return client, nil
}

// Disconnect đóng kết nối tới event store.
func Disconnect(client *kurrentdb.Client) {
	log := logger.NewLogger(zerolog.InfoLevel)
	if err := client.Close(); err != nil {
		log.Err(err).Msg("error when disconnecting from event store")
	}
}
