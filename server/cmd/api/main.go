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
	rustfs "github.com/dont-wait/anomaly/internal/infrastructure/rustfs"
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

	eventStoreClient, err := eventstore.NewEventStoreClient(config.EventStoreConfig)
	if err != nil {
		logger.Fatal().Err(err).Msg("connect event store failed")
	}
	defer eventstore.Disconnect(eventStoreClient)

	rustfsClient := rustfs.NewClient(config.RustFSConfig)
	if err := rustfs.EnsureBucket(ctx, rustfsClient, config.RustFSConfig.Bucket); err != nil {
		logger.Fatal().Err(err).Msg("ensure rustfs bucket failed")
	}
	mediaRepo := rustfs.NewMediaRepository(rustfsClient, config.RustFSConfig.Bucket)

	mongoRepo := mongo.NewAccountRepository(mongoClient, config.MongoConfig.MongoDBName)
	if err := mongoRepo.EnsureIndexes(ctx); err != nil {
		logger.Fatal().Err(err).Msg("ensure mongo indexes failed")
	}
	eventStoreRepo := eventstore.NewAccountRepository(eventStoreClient)

	tokenSvc := auth.NewTokenService(config.AuthConfig.JWTSecret, config.AuthConfig.JWTExpiry)

	accountHandler := composition.NewAccountHandler(eventStoreRepo, eventStoreRepo, tokenSvc, *logger)
	mediaHandler := composition.NewMediaHandler(mediaRepo, *logger)

	mux := netHTTP.NewServeMux()
	mux = presentation.NewRouter(mux, accountHandler, mediaHandler, tokenSvc)

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
