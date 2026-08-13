package main

import (
	"context"
	netHTTP "net/http"
	"os"
	"strings"

	"github.com/dont-wait/anomaly/internal/composition"
	mongo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	presentation "github.com/dont-wait/anomaly/internal/presentation/http"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
	"github.com/rs/zerolog"
)

func main() {
	ctx := context.Background()

	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "2006-01-02 15:04:05"}).With().Timestamp().Logger()

	mongoURI := envOr("MONGO_URI", "mongodb://localhost:27017")

	client, err := mongo.NewClient(ctx, logger, mongoURI)
	if err != nil {
		logger.Fatal().Err(err).Msg("connect mongo failed")
	}
	defer client.Disconnect(ctx)

	repo := mongo.NewAccountRepository(client, envOr("MONGO_DB", "anomaly"))

	accountHandler := composition.NewAccountHandler(repo, logger)

	mux := netHTTP.NewServeMux()
	mux = presentation.NewRouter(mux, accountHandler)

	mux.HandleFunc("GET /health", func(w netHTTP.ResponseWriter, r *netHTTP.Request) {
		w.WriteHeader(netHTTP.StatusOK)
		w.Write([]byte("OK"))
	})

	logger.Info().Msg("Anomaly Fraud Detection đang chạy tại port :8080...")
	allowedOrigins := splitCSV(envOr("CORS_ALLOWED_ORIGINS",
		"http://localhost:5173,http://localhost:3000,tauri://localhost,http://tauri.localhost"))
	if err := netHTTP.ListenAndServe(":8080", middleware.NewCORS(allowedOrigins)(mux)); err != nil {
		logger.Fatal().Err(err).Msg("server failed")
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
