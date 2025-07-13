package grpc

import (
	"context"
	"fmt"
	"sync"

	"github.com/NamiraNet/namira-core/internal/core"
	"go.uber.org/zap"
)

// distributeConfigsAcrossWorkers efficiently distributes configs across available workers
// This is the default mode - each config goes to one worker for maximum efficiency
func (g *GRPCCore) distributeConfigsAcrossWorkers(ctx context.Context, jobID string, configs []string, resultChan chan<- core.CheckResult) {
	clients, tags := g.getClientsAndTags()

	if len(clients) == 0 {
		g.handleNoClients(configs, resultChan)
		return
	}

	allResults := make(chan core.CheckResult, len(configs))
	var wg sync.WaitGroup

	g.distributeConfigsToWorkers(ctx, jobID, configs, clients, tags, allResults, &wg)
	g.collectAndForwardResults(allResults, resultChan, &wg)

	g.logger.Info("Completed distributed config check",
		zap.String("job_id", jobID),
		zap.Int("total_configs", len(configs)),
		zap.Int("workers_used", len(clients)))
}

// getClientsAndTags returns copies of clients and tags arrays
func (g *GRPCCore) getClientsAndTags() ([]*CheckerClient, []string) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	clients := make([]*CheckerClient, len(g.clients))
	tags := make([]string, len(g.clientTags))
	copy(clients, g.clients)
	copy(tags, g.clientTags)

	return clients, tags
}

// handleNoClients sends error results when no clients are available
func (g *GRPCCore) handleNoClients(configs []string, resultChan chan<- core.CheckResult) {
	g.logger.Error("No available checker clients")
	for _, config := range configs {
		resultChan <- core.CheckResult{
			Status:         core.CheckResultStatusError,
			Error:          "no available checker clients",
			Raw:            config,
			CheckerNodeTag: []string{},
		}
	}
}

// distributeConfigsToWorkers distributes configs to workers using round-robin
func (g *GRPCCore) distributeConfigsToWorkers(ctx context.Context, jobID string, configs []string, clients []*CheckerClient, tags []string, allResults chan<- core.CheckResult, wg *sync.WaitGroup) {
	for i, config := range configs {
		clientIndex := i % len(clients)
		client := clients[clientIndex]
		tag := tags[clientIndex]

		wg.Add(1)
		go g.processConfigWithWorker(ctx, jobID, config, client, tag, allResults, wg)
	}
}

// processConfigWithWorker processes a single config with a specific worker
func (g *GRPCCore) processConfigWithWorker(ctx context.Context, jobID, config string, client *CheckerClient, tag string, allResults chan<- core.CheckResult, wg *sync.WaitGroup) {
	defer wg.Done()

	workerJobID := fmt.Sprintf("%s-w%s", jobID, tag)

	g.logger.Debug("Sending config to worker",
		zap.String("worker_job_id", workerJobID),
		zap.String("worker_tag", tag),
		zap.String("config", config[:min(50, len(config))]))

	grpcResults, err := client.CheckConfigs(ctx, workerJobID, []string{config})
	if err != nil {
		g.handleWorkerError(workerJobID, tag, config, err, allResults)
		return
	}

	g.processWorkerResults(workerJobID, grpcResults, allResults)
}

// handleWorkerError handles errors from worker processing
func (g *GRPCCore) handleWorkerError(workerJobID, tag, config string, err error, allResults chan<- core.CheckResult) {
	g.logger.Error("Worker failed to process config",
		zap.String("worker_job_id", workerJobID),
		zap.String("worker_tag", tag),
		zap.Error(err))
	allResults <- core.CheckResult{
		Status:         core.CheckResultStatusError,
		Error:          err.Error(),
		Raw:            config,
		CheckerNodeTag: []string{},
	}
}

// processWorkerResults processes results from a worker
func (g *GRPCCore) processWorkerResults(workerJobID string, grpcResults <-chan *CheckerResponse, allResults chan<- core.CheckResult) {
	for result := range grpcResults {
		if result.Status != "CHECKING" && result.Status != "PENDING" {
			coreResult := g.convertToCheckResult(result)
			coreResult.CheckerNodeTag = []string{result.CheckerNodeTag}
			allResults <- coreResult

			g.logger.Debug("Worker completed config",
				zap.String("worker_job_id", workerJobID),
				zap.String("actual_worker_tag", result.CheckerNodeTag),
				zap.String("status", result.Status),
				zap.Int64("latency_ms", result.LatencyMs))
			break
		}
	}
}
