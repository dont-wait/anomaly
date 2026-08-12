package main

import (
	"context"
	"fmt"
	"log"
	netHTTP "net/http"
	"os"

	"github.com/dont-wait/anomaly/internal/composition"
	mongo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	presentation "github.com/dont-wait/anomaly/internal/presentation/http"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
)

func main() {
	ctx := context.Background()

	client, err := mongo.NewClient(ctx, envOr("MONGO_URI", "mongodb://localhost:27017"))
	if err != nil {
		log.Fatalf("connect mongo: %v", err)
	}
	defer client.Disconnect(ctx)

	repo := mongo.NewAccountRepository(client, envOr("MONGO_DB", "anomaly"))

	accountHandler := composition.NewAccountHandler(repo)

	mux := netHTTP.NewServeMux()
	mux = presentation.NewRouter(mux, accountHandler)

	mux.HandleFunc("GET /health", func(w netHTTP.ResponseWriter, r *netHTTP.Request) {
		w.WriteHeader(netHTTP.StatusOK)
		w.Write([]byte("OK"))
	})

	fmt.Println("Anomaly Fraud Detection đang chạy tại port :8080...")
	netHTTP.ListenAndServe(":8080", middleware.CORS(mux))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
