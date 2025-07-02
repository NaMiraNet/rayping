package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/NamiraNet/namira-core/internal/core"
	"github.com/NamiraNet/namira-core/internal/github"
	workerpool "github.com/NamiraNet/namira-core/internal/worker"
	"github.com/gorilla/mux"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const REDIS_SET_DURATION = 30 * time.Minute

type CallbackHandlerResult struct {
	JobID   string
	Results []core.CheckResult
	Error   error
}

type CallbackHandler func(CallbackHandlerResult)
type ConfigSuccessHandler func(core.CheckResult)

type Handler struct {
	core                  *core.Core
	workerPool            *workerpool.WorkerPool
	redis                 *redis.Client
	jobs                  sync.Map
	logger                *zap.Logger
	updater               *github.Updater
	jobsOnSuccess         ConfigSuccessHandler
	versionInfo           VersionInfo
	redisResultExpiration time.Duration

	// Background refresh components
	refreshMutex    sync.RWMutex
	refreshTicker   *time.Ticker
	refreshInterval time.Duration
	refreshDone     chan struct{}
}

func NewHandler(c *core.Core, redisClient *redis.Client, callbackHandler CallbackHandler, configSuccessHandler ConfigSuccessHandler, logger *zap.Logger, updater *github.Updater, worker *workerpool.WorkerPool, versionInfo VersionInfo, redisResultExpiration time.Duration, refreshInterval time.Duration) *Handler {
	handler := &Handler{
		core:                  c,
		workerPool:            worker,
		redis:                 redisClient,
		logger:                logger,
		updater:               updater,
		jobsOnSuccess:         configSuccessHandler,
		versionInfo:           versionInfo,
		redisResultExpiration: redisResultExpiration,
		refreshInterval:       refreshInterval,
		refreshDone:           make(chan struct{}),
	}

	worker.SetResultHandler(handler.handleTaskResult(callbackHandler))
	if err := worker.Start(); err != nil {
		panic("Failed to start worker pool: " + err.Error())
	}

	// Start background refresh
	go handler.startBackgroundRefresh()

	return handler
}

func (h *Handler) startBackgroundRefresh() {
	h.refreshTicker = time.NewTicker(h.refreshInterval)

	go func() {
		h.logger.Info("Background refresh started", zap.Duration("interval", h.refreshInterval))
		for {
			select {
			case <-h.refreshTicker.C:
				h.performBackgroundRefresh()
			case <-h.refreshDone:
				h.refreshTicker.Stop()
				h.logger.Info("Background refresh stopped")
				return
			}
		}
	}()
}

func (h *Handler) performBackgroundRefresh() {
	h.logger.Info("Starting background refresh of all configurations")

	// Acquire write lock - blocks all API operations
	h.refreshMutex.Lock()
	defer h.refreshMutex.Unlock()

	start := time.Now()

	configs, err := h.updater.GetCurrentConfigs()
	if err != nil {
		h.logger.Error("Failed to fetch current configs during refresh", zap.Error(err))
		return
	}

	if len(configs) == 0 {
		h.logger.Info("No existing configs found, skipping refresh")
		return
	}

	if err := h.flushRedisCache(); err != nil {
		h.logger.Error("Failed to flush Redis cache", zap.Error(err))
		return
	}

	job := NewJob(configs)
	job.ID = "refresh-" + job.ID // Mark as refresh job
	h.jobs.Store(job.ID, job)
	job.Start()

	if err := h.workerPool.Submit(workerpool.Task{
		ID:      job.ID,
		Data:    TaskData{JobID: job.ID, Configs: configs},
		Execute: h.executeCheckTask,
		Callback: func(res interface{}, _ error) {
			result := res.(CallbackHandlerResult)
			if result.Error != nil {
				h.logger.Error("Failed to execute refresh task", zap.Error(result.Error))
				job.Fail(result.Error)
				return
			}
			job.Complete()

			h.logger.Info("Background refresh completed",
				zap.Duration("duration", time.Since(start)),
				zap.Int("configs_refreshed", len(configs)),
				zap.String("job_id", job.ID))
		},
	}); err != nil {
		h.logger.Error("Failed to submit refresh task", zap.Error(err))
		job.Fail(err)
		return
	}
}

func (h *Handler) handleScan(w http.ResponseWriter, r *http.Request) {
	// Acquire read lock - allows concurrent API calls but blocks during refresh
	if !h.refreshMutex.TryRLock() {
		h.logger.Info("Background refresh in progress, skipping scan")
		writeError(w, "Background refresh in progress", http.StatusServiceUnavailable)
		return
	}
	defer h.refreshMutex.RUnlock()

	var configs []string

	// Check content type
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "multipart/form-data") {
		file, _, err := r.FormFile("file")
		if err != nil {
			h.logger.Error("Failed to read file", zap.Error(err))
			writeError(w, "Failed to read file", http.StatusBadRequest)
			return
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				configs = append(configs, line)
			}
		}
		if err := scanner.Err(); err != nil {
			h.logger.Error("Failed to read file content", zap.Error(err))
			writeError(w, "Failed to read file content", http.StatusBadRequest)
			return
		}
	} else {
		// Handle JSON request
		var req ScanRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.logger.Error("Invalid Json", zap.Error(err))
			writeError(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		configs = req.Configs
	}

	if len(configs) == 0 {
		h.logger.Error("No configs provided")
		writeError(w, "No configs provided", http.StatusBadRequest)
		return
	}

	uniqueConfigs, err := h.filterDuplicates(configs)
	if err != nil {
		h.logger.Error("Failed to filter duplicates", zap.Error(err))
		writeError(w, "Failed to filter duplicates", http.StatusInternalServerError)
		return
	}

	if len(uniqueConfigs) == 0 {
		h.logger.Error("All configs are duplicates")
		writeError(w, "All configs are duplicates", http.StatusBadRequest)
		return
	}

	job := NewJob(uniqueConfigs)
	h.jobs.Store(job.ID, job)
	job.Start()

	if err := h.workerPool.Submit(workerpool.Task{
		ID:      job.ID,
		Data:    TaskData{JobID: job.ID, Configs: uniqueConfigs},
		Execute: h.executeCheckTask,
	}); err != nil {
		h.logger.Error("Failed to submit task", zap.Error(err))
		job.Fail(err)
		writeError(w, "Failed to submit task", http.StatusInternalServerError)
		return
	}

	writeJSON(w, ScanResponse{JobID: job.ID})
}

func (h *Handler) handleJobStatus(w http.ResponseWriter, r *http.Request) {
	if value, exists := h.jobs.Load(mux.Vars(r)["id"]); exists {
		writeJSON(w, value.(*Job))
		return
	}
	h.logger.Error("Job not found")
	writeError(w, "Job not found", http.StatusNotFound)
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	stats := h.workerPool.GetStats()
	writeJSON(w, HealthResponse{
		Status:  "ok",
		Version: h.versionInfo.Version,
		Build: BuildInfo{
			Version:   h.versionInfo.Version,
			Commit:    h.versionInfo.Commit,
			Date:      h.versionInfo.Date,
			GoVersion: h.versionInfo.GoVersion,
			Platform:  h.versionInfo.Platform,
		},
		WorkerPool: WorkerPoolStatus{
			WorkerCount:    stats.WorkerCount,
			TotalTasks:     stats.TotalTasks,
			CompletedTasks: stats.CompletedTasks,
			FailedTasks:    stats.FailedTasks,
			QueueLength:    stats.QueueLength,
			IsRunning:      stats.IsRunning,
			Uptime:         stats.Uptime.String(),
		},
	})
}

func (h *Handler) flushRedisCache() error {
	const (
		pattern   = "config:*"
		batchSize = 1000
	)

	ctx := context.Background()
	iter := h.redis.Scan(ctx, 0, pattern, 0).Iterator()
	pipe := h.redis.Pipeline()

	batch := make([]string, 0, batchSize)
	for iter.Next(ctx) {
		batch = append(batch, iter.Val())

		if len(batch) == batchSize {
			if err := pipe.Del(ctx, batch...).Err(); err != nil {
				return fmt.Errorf("failed to delete batch from Redis: %w", err)
			}
			batch = batch[:0]
		}
	}

	if len(batch) > 0 {
		if err := pipe.Del(ctx, batch...).Err(); err != nil {
			return fmt.Errorf("failed to delete remaining keys from Redis: %w", err)
		}
	}

	if err := iter.Err(); err != nil {
		return fmt.Errorf("failed to iterate Redis keys: %w", err)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to execute Redis pipeline: %w", err)
	}

	return nil
}

func (h *Handler) executeCheckTask(ctx context.Context, data interface{}) (interface{}, error) {
	taskData := data.(TaskData)
	value, exists := h.jobs.Load(taskData.JobID)
	if !exists {
		h.logger.Error("Job not found", zap.String("job_id", taskData.JobID))
		return CallbackHandlerResult{
			JobID: taskData.JobID,
			Error: fmt.Errorf("job not found: %s", taskData.JobID),
		}, nil
	}

	job := value.(*Job)
	results := make([]core.CheckResult, 0, len(taskData.Configs))

	i := 0
	for result := range h.core.CheckConfigs(taskData.Configs) {
		results = append(results, result)
		checkResult := CheckResult{
			Index:  i,
			Status: string(result.Status),
			Delay:  result.RealDelay.Milliseconds(),
		}

		if result.Error != "" {
			h.logger.Error("Config check failed",
				zap.String("error", result.Error),
				zap.String("status", string(result.Status)),
				zap.String("server", result.Server),
				zap.String("protocol", result.Protocol))
			job.Done()
		} else {
			h.logger.Info("Config check succeeded",
				zap.String("server", result.Server),
				zap.String("protocol", result.Protocol),
				zap.Int64("delay_ms", result.RealDelay.Milliseconds()))

			job.AddResult(HashConfig(result.Raw), checkResult)
			if !strings.HasPrefix(job.ID, "refresh-") {
				h.jobsOnSuccess(result)
			}
		}

		if job.DoneCount >= job.TotalCount {
			job.Complete()
			h.logger.Info("Job processing completed",
				zap.String("job_id", taskData.JobID),
				zap.Int("total_processed", job.TotalCount))
		}
		i++
	}

	return CallbackHandlerResult{
		JobID:   taskData.JobID,
		Results: results,
	}, nil
}

func (h *Handler) handleTaskResult(callback CallbackHandler) func(workerpool.Result) {
	return func(result workerpool.Result) {
		if result.Error == nil {
			callbackResult := result.Result.(CallbackHandlerResult)

			// Store results in Redis
			scanResult := github.ScanResult{
				JobID:     callbackResult.JobID,
				Results:   callbackResult.Results,
				Timestamp: time.Now(),
			}

			resultsData, err := json.Marshal(scanResult)
			if err != nil {
				h.logger.Error("Failed to marshal scan results", zap.Error(err))
				return
			}

			ctx := context.Background()
			resultsKey := fmt.Sprintf("scan_results:%s", callbackResult.JobID)
			if err := h.redis.Set(ctx, resultsKey, resultsData, REDIS_SET_DURATION).Err(); err != nil {
				h.logger.Error("Failed to store results in Redis", zap.Error(err))
				return
			}

			callback(callbackResult)
		}
	}
}

func (h *Handler) filterDuplicates(configs []string) ([]string, error) {
	ctx := context.Background()
	uniqueConfigs := make([]string, 0, len(configs))
	pipe := h.redis.Pipeline()
	cmds := make([]*redis.IntCmd, len(configs))
	hashes := make([]string, len(configs))

	for i, config := range configs {
		hash := HashConfig(config)
		hashes[i] = hash
		cmds[i] = pipe.Exists(ctx, "config:"+hash)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		h.logger.Error("Failed to filter duplicates", zap.Error(err))
		return nil, err
	}

	pipe = h.redis.Pipeline()
	for i, cmd := range cmds {
		if cmd.Val() == 0 {
			uniqueConfigs = append(uniqueConfigs, configs[i])
			pipe.Set(ctx, "config:"+hashes[i], "1", h.redisResultExpiration)
		}
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		h.logger.Error("Failed to filter duplicates", zap.Error(err))
		return nil, err
	}
	return uniqueConfigs, err
}

func (h *Handler) Close() {
	h.workerPool.Stop()
}

// Helper functions
func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(MessageResponse{
		Status:  code,
		Message: message,
	}); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
