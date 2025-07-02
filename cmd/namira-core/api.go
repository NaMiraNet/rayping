package main

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"

	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NamiraNet/namira-core/internal/api"
	"github.com/NamiraNet/namira-core/internal/core"
	"github.com/NamiraNet/namira-core/internal/github"
	"github.com/NamiraNet/namira-core/internal/logger"
	"github.com/NamiraNet/namira-core/internal/notify"
	workerpool "github.com/NamiraNet/namira-core/internal/worker"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Start the API server",
	Long:  `Start the namira-core API server to handle VPN link checking requests.`,
	Run:   runAPIServer,
}

func runAPIServer(cmd *cobra.Command, args []string) {
	logger, err := logger.InitForAPI(cfg.App.LogLevel, true)
	if err != nil {
		fmt.Println("Failed to initialize logger:", err)
		os.Exit(1)
	}
	defer func() {
		_ = logger.Sync()
	}()

	// Use global config, but allow CLI flags to override
	checkServer, checkP, err := net.SplitHostPort(cfg.App.CheckHost)
	if err != nil {
		logger.Fatal("Failed to parse check server", zap.Error(err))
	}
	checkPort, _ := strconv.Atoi(checkP)

	// Initialize Redis client
	redisClient = redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Fatal("Failed to connect to Redis", zap.Error(err))
	}
	logger.Info("Connected to Redis successfully", zap.String("addr", cfg.Redis.Addr))

	// Initialize GitHub updater
	encryptionKey := []byte(cfg.App.EncryptionKey)
	updater, err = github.NewUpdater(
		logger,
		cfg.Github.SSHKeyPath,
		redisClient,
		cfg.Github.Owner,
		cfg.Github.Repo,
		encryptionKey,
	)
	if err != nil {
		logger.Fatal("Failed to create updater:", zap.Error(err))
	}

	if err := updater.HealthCheck(); err != nil {
		logger.Fatal("GitHub SSH connectivity test failed:", zap.Error(err))
	}

	logger.Info("GitHub updater initialized successfully",
		zap.String("repo", fmt.Sprintf("%s/%s", cfg.Github.Owner, cfg.Github.Repo)),
		zap.String("ssh_key", cfg.Github.SSHKeyPath))

	coreInstance := core.NewCore(core.CoreOpts{
		CheckTimeout:       cfg.App.Timeout,
		CheckServer:        checkServer,
		CheckPort:          uint32(checkPort),
		CheckMaxConcurrent: cfg.App.MaxConcurrent,
		RemarkTemplate: &core.RemarkTemplate{
			OrgName:      "@NamiraNet",
			Separator:    " | ",
			ShowCountry:  true,
			ShowHost:     true,
			ShowProtocol: true,
		},
	})

	callbackHandler := func(result api.CallbackHandlerResult) {
		if result.Error != nil {
			logger.Error("Job failed",
				zap.String("job_id", result.JobID),
				zap.Error(result.Error))
			return
		}

		logger.Info("Job completed successfully",
			zap.String("job_id", result.JobID),
			zap.Int("total_results", len(result.Results)),
			zap.Int("valid_configs", countValidConfigs(result.Results)))

		// Determine job type and handle accordingly
		if strings.HasPrefix(result.JobID, "refresh-") {
			// Background refresh job - update GitHub with all results
			if err := updater.ProcessRefreshResults(result.JobID); err != nil {
				logger.Error("Failed to process refresh results",
					zap.String("job_id", result.JobID),
					zap.Error(err))
			} else {
				logger.Info("Refresh results processed successfully",
					zap.String("job_id", result.JobID))
			}
		} else {
			// Regular scan job - merge with existing configs
			if err := updater.ProcessScanResults(result.JobID); err != nil {
				logger.Error("Failed to process scan results",
					zap.String("job_id", result.JobID),
					zap.Error(err))
			} else {
				logger.Info("Scan results processed successfully",
					zap.String("job_id", result.JobID))
			}
		}
	}

	telegramTransport := &http.Transport{}
	if proxyURL := cfg.Telegram.ProxyURL; proxyURL != "" {
		proxy, err := url.Parse(proxyURL)
		if err != nil {
			logger.Fatal("Failed to parse proxy URL", zap.Error(err))
		}
		telegramTransport.Proxy = http.ProxyURL(proxy)
	}

	telegram := notify.NewTelegram(
		cfg.Telegram.BotToken,
		cfg.Telegram.Channel,
		cfg.Telegram.Template,
		cfg.Telegram.QRConfig,
		&http.Client{
			Timeout:   10 * time.Second,
			Transport: telegramTransport,
		},
	)

	tgLimiter := rate.NewLimiter(rate.Every(cfg.Telegram.SendingInterval), 1)

	telegramConfigResultHandler := func(result core.CheckResult) {
		// ignore if error or delay too long
		if result.Error != "" || result.RealDelay > 3*time.Second {
			return
		}
		go func() {
			if err := tgLimiter.Wait(context.Background()); err != nil {
				logger.Error("Rate limit error", zap.Error(err))
				return
			}

			if err := telegram.SendWithQRCode(result); err != nil {
				logger.Error("Failed to send Telegram notification", zap.Error(err))
			}
		}()
	}
	// worker instace
	worker := workerpool.NewWorkerPool(workerpool.WorkerPoolConfig{
		WorkerCount:   cfg.Worker.Count,
		TaskQueueSize: cfg.Worker.QueueSize,
	})

	versionInfo := api.VersionInfo{
		Version:   version,
		Commit:    commit,
		Date:      date,
		GoVersion: goVersion,
		Platform:  platform,
	}

	router := api.NewRouter(
		coreInstance,
		redisClient,
		callbackHandler,
		telegramConfigResultHandler,
		logger,
		updater,
		worker,
		versionInfo,
		cfg.Redis.ResultTTL,
		cfg.App.RefreshInterval)

	serverAddr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
	server := &http.Server{
		Addr:         serverAddr,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	go func() {
		logger.Info("Server starting",
			zap.String("address", server.Addr),
			zap.Duration("read_timeout", cfg.Server.ReadTimeout),
			zap.Duration("write_timeout", cfg.Server.WriteTimeout),
		)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server failed to start", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Server shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}
}

// Helper function to count valid configurations
func countValidConfigs(results []core.CheckResult) int {
	count := 0
	for _, result := range results {
		if result.Error == "" && result.Status == core.CheckResultStatusSuccess {
			count++
		}
	}
	return count
}
