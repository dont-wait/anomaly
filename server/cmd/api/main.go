package main

import (
	"context"
	netHTTP "net/http"
	"time"

	"github.com/dont-wait/anomaly/internal/composition"
	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/helpers"
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
	mongoConf := loader.LoadMongoConfig()

	client, err := mongo.NewMongoClient(ctx, mongoConf)
	if err != nil {
		logger.Fatal().Err(err).Msg("connect mongo failed")
	}
	defer client.Disconnect(ctx)

	repo := mongo.NewAccountRepository(client, mongoConf.MongoDBName)

	accountHandler := composition.NewAccountHandler(repo, *logger)

	mux := netHTTP.NewServeMux()
	mux = presentation.NewRouter(mux, accountHandler)

	mux.HandleFunc("GET /health", func(w netHTTP.ResponseWriter, r *netHTTP.Request) {
		w.WriteHeader(netHTTP.StatusOK)
		w.Write([]byte("OK"))
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
