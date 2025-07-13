package grpc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/NamiraNet/namira-core/internal/core"
	"go.uber.org/zap"
)

// processConfigWithAllWorkers sends a single config to all worker nodes and aggregates results
func (g *GRPCCore) processConfigWithAllWorkers(ctx context.Context, jobID, config string) core.CheckResult {
	clients, tags := g.getClientsAndTags()

	if len(clients) == 0 {
		return g.createErrorResult("no available checker clients", config, []string{})
	}

	resultChan := make(chan workerResult, len(clients))
	g.sendConfigToAllWorkers(ctx, jobID, config, clients, tags, resultChan)

	return g.collectAndAggregateResults(ctx, jobID, config, clients, resultChan)
}

// workerResult represents the result from a single worker
type workerResult struct {
	tag    string
	result *CheckerResponse
	err    error
}

// sendConfigToAllWorkers sends the config to all workers in parallel
func (g *GRPCCore) sendConfigToAllWorkers(ctx context.Context, jobID, config string, clients []*CheckerClient, tags []string, resultChan chan<- workerResult) {
	for i, client := range clients {
		go g.processConfigWithSingleWorker(ctx, jobID, config, client, tags[i], i, resultChan)
	}
}

// processConfigWithSingleWorker processes config with a single worker
func (g *GRPCCore) processConfigWithSingleWorker(ctx context.Context, jobID, config string, client *CheckerClient, tag string, workerIndex int, resultChan chan<- workerResult) {
	workerJobID := fmt.Sprintf("%s-w%d-%s", jobID, workerIndex, tag)

	grpcResults, err := client.CheckConfigs(ctx, workerJobID, []string{config})
	if err != nil {
		resultChan <- workerResult{tag: tag, err: err}
		return
	}

	for result := range grpcResults {
		if result.Status != "CHECKING" && result.Status != "PENDING" {
			resultChan <- workerResult{tag: result.CheckerNodeTag, result: result}
			return
		}
	}

	resultChan <- workerResult{tag: tag, err: fmt.Errorf("no result received")}
}

// collectAndAggregateResults collects results from all workers and aggregates them
func (g *GRPCCore) collectAndAggregateResults(ctx context.Context, jobID, config string, clients []*CheckerClient, resultChan <-chan workerResult) core.CheckResult {
	var successfulResults []workerResult
	var totalLatency int64
	var successfulTags []string

	for i := 0; i < len(clients); i++ {
		select {
		case result := <-resultChan:
			if result.err != nil {
				g.logWorkerError(jobID, result.tag, config, result.err)
				continue
			}

			if g.isSuccessfulResult(result.result) {
				successfulResults = append(successfulResults, result)
				totalLatency += result.result.LatencyMs
				successfulTags = append(successfulTags, result.result.CheckerNodeTag)
				g.logWorkerSuccess(jobID, result.result)
			} else {
				g.logWorkerFailure(jobID, result.tag, result.result)
			}
		case <-ctx.Done():
			return g.createErrorResult("timeout waiting for worker results", config, successfulTags)
		}
	}

	return g.buildAggregatedResult(jobID, config, successfulResults, totalLatency, successfulTags, len(clients))
}

// isSuccessfulResult checks if a worker result is successful
func (g *GRPCCore) isSuccessfulResult(result *CheckerResponse) bool {
	return result.IsValid && result.Status == "SUCCESS"
}

// createErrorResult creates a CheckResult with error status
func (g *GRPCCore) createErrorResult(errorMsg, config string, tags []string) core.CheckResult {
	return core.CheckResult{
		Status:         core.CheckResultStatusError,
		Error:          errorMsg,
		Raw:            config,
		CheckerNodeTag: tags,
	}
}

// buildAggregatedResult builds the final aggregated result
func (g *GRPCCore) buildAggregatedResult(jobID, config string, successfulResults []workerResult, totalLatency int64, successfulTags []string, totalWorkers int) core.CheckResult {
	if len(successfulResults) == 0 {
		return g.createErrorResult("all workers failed to validate config", config, []string{})
	}

	baseResult := successfulResults[0].result
	avgLatency := totalLatency / int64(len(successfulResults))

	g.logger.Info("Aggregated config check result",
		zap.String("job_id", jobID),
		zap.Int("successful_workers", len(successfulResults)),
		zap.Int("total_workers", totalWorkers),
		zap.Int64("avg_latency_ms", avgLatency),
		zap.Strings("successful_tags", successfulTags))

	return core.CheckResult{
		Status:         core.CheckResultStatusSuccess,
		Protocol:       baseResult.Protocol,
		Raw:            baseResult.Config,
		Server:         baseResult.Server,
		CountryCode:    baseResult.CountryCode,
		Remark:         baseResult.Remark,
		RealDelay:      time.Duration(avgLatency) * time.Millisecond,
		CheckerNodeTag: successfulTags,
	}
}

// logWorkerError logs worker errors
func (g *GRPCCore) logWorkerError(jobID, tag, config string, err error) {
	g.logger.Warn("Worker failed to process config",
		zap.String("job_id", jobID),
		zap.String("configured_worker_tag", tag),
		zap.String("config", config[:min(50, len(config))]),
		zap.Error(err))
}

// logWorkerSuccess logs successful worker results
func (g *GRPCCore) logWorkerSuccess(jobID string, result *CheckerResponse) {
	g.logger.Debug("Worker successfully processed config",
		zap.String("job_id", jobID),
		zap.String("actual_worker_tag", result.CheckerNodeTag),
		zap.Int64("latency_ms", result.LatencyMs),
		zap.String("status", result.Status))
}

// logWorkerFailure logs worker failures
func (g *GRPCCore) logWorkerFailure(jobID, tag string, result *CheckerResponse) {
	g.logger.Debug("Worker failed to validate config",
		zap.String("job_id", jobID),
		zap.String("configured_worker_tag", tag),
		zap.String("actual_worker_tag", result.CheckerNodeTag),
		zap.String("status", result.Status),
		zap.String("error", result.Error))
}

// collectAndForwardResults waits for all workers and forwards results
func (g *GRPCCore) collectAndForwardResults(allResults chan core.CheckResult, resultChan chan<- core.CheckResult, wg *sync.WaitGroup) {
	go func() {
		wg.Wait()
		close(allResults)
	}()

	for result := range allResults {
		resultChan <- result
	}
}

// processConfigsWithAllWorkers processes multiple configs in parallel, sending each to all workers
func (g *GRPCCore) processConfigsWithAllWorkers(ctx context.Context, jobID string, configs []string, resultChan chan<- core.CheckResult) {
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, g.maxConcurrent)

	for i, config := range configs {
		wg.Add(1)
		go func(configIndex int, config string) {
			defer wg.Done()

			// Acquire semaphore to limit concurrent processing
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			configJobID := fmt.Sprintf("%s-c%d", jobID, configIndex)
			result := g.processConfigWithAllWorkers(ctx, configJobID, config)
			resultChan <- result
		}(i, config)
	}

	wg.Wait()
}
