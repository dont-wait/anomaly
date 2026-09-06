package eventstore

import (
	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/logger"
	eventstoredb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
	"github.com/rs/zerolog"
)

// tạo kết nối tới EventStoreDB.
func NewEventStoreClient(conf *domain.EventStoreConfig) (*eventstoredb.Client, error) {
	log := logger.NewLogger(zerolog.InfoLevel)

	settings, err := eventstoredb.ParseConnectionString(conf.EventStoreConnString)
	if err != nil {
		return nil, err
	}
	client, err := eventstoredb.NewClient(settings)
	if err != nil {
		return nil, err
	}
	log.Info().Msg("Connected to event store")
	return client, nil
}

// Disconnect đóng kết nối tới event store.
func Disconnect(client *eventstoredb.Client) {
	log := logger.NewLogger(zerolog.InfoLevel)
	if err := client.Close(); err != nil {
		log.Err(err).Msg("error when disconnecting from event store")
	}
}
