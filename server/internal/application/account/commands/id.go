package commands

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/dont-wait/anomaly/internal/logger"
	"github.com/rs/zerolog"
)

func newID() string {
	logger := logger.NewLogger(zerolog.Level(zerolog.InfoLevel))
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		logger.Info().Msgf("err: %v", err)
	}
	return hex.EncodeToString(b)
}
