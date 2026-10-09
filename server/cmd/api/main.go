package main

import (
	"context"
	netHTTP "net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/dont-wait/anomaly/internal/composition"
	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/helpers"
	"github.com/dont-wait/anomaly/internal/infrastructure/auth"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	"github.com/dont-wait/anomaly/internal/infrastructure/kyc"
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

	rustfsClient := rustfs.NewClient(config.RustFSConfig)
	if err := rustfs.EnsureBucket(ctx, rustfsClient, config.RustFSConfig.Bucket); err != nil {
		logger.Fatal().Err(err).Msg("ensure rustfs bucket failed")
	}
	mediaRepo := rustfs.NewMediaRepository(rustfsClient, config.RustFSConfig.Bucket)

	esClient, err := eventstore.NewEventStoreClient(config.EventStoreConfig)
	if err != nil {
		logger.Fatal().Err(err).Msg("connect eventstore failed")
	}
	defer eventstore.Disconnect(esClient)
	bank := eventstore.NewBankRepository(esClient, mongo.NewTransferRepository(mongoClient, config.MongoConfig.MongoDBName))
	legacyAccounts, err := mongoClient.Database(config.MongoConfig.MongoDBName).Collection("accounts").EstimatedDocumentCount(ctx)
	if err != nil {
		logger.Fatal().Err(err).Msg("check existing account data")
	}
	if err := bank.EnsureReady(ctx, legacyAccounts > 0); err != nil {
		logger.Fatal().Err(err).Msg("banking source of truth unavailable")
	}

	tokenSvc := auth.NewTokenService(config.AuthConfig.JWTSecret, config.AuthConfig.JWTExpiry)

	rdb := redis.NewClient(&redis.Options{
		Addr:     config.RedisConfig.Addr,
		Password: config.RedisConfig.Password,
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Fatal().Err(err).Msg("connect redis failed")
	}
	logger.Info().Str("addr", config.RedisConfig.Addr).Msg("connected to redis")
	defer func() {
		if err := rdb.Close(); err != nil {
			logger.Error().Err(err).Msg("disconnect redis failed")
		}
	}()

	accountHandler := composition.NewAccountHandlerWithKYC(bank, tokenSvc, kyc.NewClient(config.KYCConfig.ServiceURL), mediaRepo, *logger)
	mediaHandler := composition.NewMediaHandler(mediaRepo, bank, *logger)
	otpHandler := composition.NewOTPHandler(rdb, config.SMTPConfig, *logger)
	transactionHandler := composition.NewTransactionHandler(bank, rdb, config.SMTPConfig, *logger)

	mux := netHTTP.NewServeMux()
	mux = presentation.NewRouter(mux, accountHandler, mediaHandler, otpHandler, transactionHandler, tokenSvc, config.DocsConfig.SwaggerEnabled)

	logger.Info().Msg("Anomaly Fraud Detection running on port :8080...")
	allowedOrigins := helpers.SplitCSV(loader.LoadEnvOr(
		"CORS_ALLOWED_ORIGINS",
		"http://localhost:1420,http://localhost:1422,http://localhost:5173,http://localhost:3000,tauri://localhost,http://tauri.localhost",
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
