package main

import (
	"fmt"
	netHTTP "net/http"

	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
)

func main() {
	netHTTP.HandleFunc("/health", func(w netHTTP.ResponseWriter, r *netHTTP.Request) {
		w.WriteHeader(netHTTP.StatusOK)
		w.Write([]byte("OK"))
	})

	fmt.Println("🏦 Anomaly Fraud Detection đang chạy tại port :8080...")
	netHTTP.ListenAndServe(":8080", middleware.CORS(netHTTP.DefaultServeMux))
}
