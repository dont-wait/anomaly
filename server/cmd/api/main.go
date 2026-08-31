package main

import (
	"context"
	netHTTP "net/http"
	"time"

	"github.com/dont-wait/anomaly/internal/composition"
	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/helpers"
	"github.com/dont-wait/anomaly/internal/infrastructure/auth"
	eventstore "github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	mongo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/dont-wait/anomaly/internal/logger"
	presentation "github.com/dont-wait/anomaly/internal/presentation/http"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
	"github.com/rs/zerolog"
)

func main() {
	ctx := context.Background()

	logger := logger.NewLogger(zerolog.InfoLevel)

	loader := domain.GetEnvLoader().Load(logger)
	config := loader.LoadAllConfig()

	mongoClient, err := mongo.NewMongoClient(ctx, config.MongoConfig)
	if err != nil {
		logger.Fatal().Err(err).Msg("connect mongo failed")
	}
	defer func() {
		if err := mongoClient.Disconnect(ctx); err != nil {
			logger.Error().Err(err).Msg("disconnect mongo failed")
		}
	}()

	esClient, err := eventstore.NewEventStoreClient(config.EventStoreConfig)
	if err != nil {
		logger.Fatal().Err(err).Msg("connect event store failed")
	}
	defer eventstore.Disconnect(esClient)

	mongoRepo := mongo.NewAccountRepository(mongoClient, config.MongoConfig.MongoDBName)
	esRepo := eventstore.NewAccountRepository(esClient)

	tokenSvc := auth.NewTokenService(config.AuthConfig.JWTSecret, config.AuthConfig.JWTExpiry)

	accountHandler := composition.NewAccountHandler(esRepo, mongoRepo, tokenSvc, *logger)

	mux := netHTTP.NewServeMux()
	mux = presentation.NewRouter(mux, accountHandler, tokenSvc)

	mux.HandleFunc("GET /health", func(w netHTTP.ResponseWriter, r *netHTTP.Request) {
		w.WriteHeader(netHTTP.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	logger.Info().Msg("Anomaly Fraud Detection running on port :8080...")
	allowedOrigins := helpers.SplitCSV(loader.LoadEnvOr(
		"CORS_ALLOWED_ORIGINS",
		"http://localhost:1420,http://localhost:5173,http://localhost:3000,tauri://localhost,http://tauri.localhost",
	))
	srv := &netHTTP.Server{
		Addr:              ":8080",
		Handler:           middleware.NewCORS(allowedOrigins)(mux),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		logger.Fatal().Err(err).Msg("server failed")
	}
}
