package api

import (
	"net/http"
	"os"
	"time"

	"github.com/NamiraNet/namira-core/internal/core"
	"github.com/NamiraNet/namira-core/internal/github"
	"github.com/NamiraNet/namira-core/internal/logger"
	workerpool "github.com/NamiraNet/namira-core/internal/worker"
	"github.com/gorilla/mux"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func NewRouter(c *core.Core, redisClient *redis.Client, callbackHandler CallbackHandler, configSuccessHandler ConfigSuccessHandler, logger *zap.Logger, updater *github.Updater, worker *workerpool.WorkerPool, versionInfo VersionInfo, redisResultExpiration time.Duration, refreshInterval time.Duration) *mux.Router {
	r := mux.NewRouter()
	h := NewHandler(c, redisClient, callbackHandler, configSuccessHandler, logger, updater, worker, versionInfo, redisResultExpiration, refreshInterval)

	r.Use(corsMiddleware, authMiddleware, loggingMiddleware)

	r.HandleFunc("/scan", h.handleScan).Methods(http.MethodPost)
	r.HandleFunc("/job/{id}", h.handleJobStatus).Methods(http.MethodGet)
	r.HandleFunc("/health", h.handleHealth).Methods(http.MethodGet)

	return r
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Info("request received",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.String("remote_addr", r.RemoteAddr),
		)
		next.ServeHTTP(w, r)
	})
}

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health check endpoint
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		apiKey := r.Header.Get("X-API-Key")
		expectedAPIKey := os.Getenv("API_KEY")

		if expectedAPIKey == "" {
			logger.Error("API_KEY environment variable not set")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if apiKey != expectedAPIKey {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
