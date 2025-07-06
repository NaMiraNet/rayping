package grpc

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/NamiraNet/namira-core/internal/core"
	"go.uber.org/zap"
)

type GRPCCore struct {
	client         *CheckerClient
	logger         *zap.Logger
	timeout        time.Duration
	maxConcurrent  int
	totalRequests  atomic.Int64
	activeRequests atomic.Int32
}

type GRPCCoreOpts struct {
	CheckerServiceAddr string
	Timeout            time.Duration
	MaxConcurrent      int
	Logger             *zap.Logger
}

func NewGRPCCore(opts GRPCCoreOpts) (*GRPCCore, error) {
	opts.Timeout = opts.Timeout.Truncate(time.Second)
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.MaxConcurrent <= 0 {
		opts.MaxConcurrent = 100
	}
	if opts.Logger == nil {
		opts.Logger = zap.NewNop()
	}

	client, err := NewCheckerClient(opts.CheckerServiceAddr, opts.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create checker client: %w", err)
	}

	return &GRPCCore{
		client:        client,
		logger:        opts.Logger,
		timeout:       opts.Timeout,
		maxConcurrent: opts.MaxConcurrent,
	}, nil
}

func (g *GRPCCore) CheckConfigs(configs []string) <-chan core.CheckResult {
	resultChan := make(chan core.CheckResult, len(configs))

	go g.processConfigs(configs, resultChan)

	return resultChan
}

func (g *GRPCCore) processConfigs(configs []string, resultChan chan<- core.CheckResult) {
	defer close(resultChan)

	g.totalRequests.Add(1)
	g.activeRequests.Add(1)
	defer g.activeRequests.Add(-1)

	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()

	jobID := fmt.Sprintf("grpc-%d", time.Now().UnixNano())

	g.logger.Info("Starting gRPC config check",
		zap.String("job_id", jobID),
		zap.Int("config_count", len(configs)))

	grpcResults, err := g.client.CheckConfigs(ctx, jobID, configs)
	if err != nil {
		g.handleError(err, configs, resultChan, jobID)
		return
	}

	g.processResults(grpcResults, resultChan, jobID)
}

func (g *GRPCCore) handleError(err error, configs []string, resultChan chan<- core.CheckResult, jobID string) {
	g.logger.Error("Failed to start gRPC config checking",
		zap.String("job_id", jobID),
		zap.Error(err))

	errResult := core.CheckResult{
		Status: core.CheckResultStatusError,
		Error:  err.Error(),
	}

	for _, config := range configs {
		errResult.Raw = config
		resultChan <- errResult
	}
}

func (g *GRPCCore) processResults(grpcResults <-chan *CheckerResponse, resultChan chan<- core.CheckResult, jobID string) {
	processed := 0
	for result := range grpcResults {
		if result.Status == "CHECKING" || result.Status == "PENDING" {
			continue
		}

		resultChan <- g.convertToCheckResult(result)
		processed++
	}

	g.logger.Info("Completed gRPC config check",
		zap.String("job_id", jobID),
		zap.Int("processed_count", processed))
}

func (g *GRPCCore) CheckConfigsList(configs []string) []core.CheckResult {
	results := make([]core.CheckResult, 0, len(configs))

	for result := range g.CheckConfigs(configs) {
		results = append(results, result)
	}

	return results
}

func (g *GRPCCore) HealthCheck(ctx context.Context) error {
	return g.client.HealthCheck(ctx)
}

func (g *GRPCCore) GetStats(ctx context.Context) (*GRPCCoreStats, error) {
	stats, err := g.client.GetStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	return &GRPCCoreStats{
		TotalRequests:  g.totalRequests.Load(),
		ActiveRequests: g.activeRequests.Load(),
		RemoteStats:    stats,
	}, nil
}

func (g *GRPCCore) Close() error {
	return g.client.Close()
}

func (g *GRPCCore) convertToCheckResult(grpcResult *CheckerResponse) core.CheckResult {
	result := core.CheckResult{
		Raw:         grpcResult.Config,
		Protocol:    grpcResult.Protocol,
		Server:      grpcResult.Server,
		CountryCode: grpcResult.CountryCode,
		Remark:      grpcResult.Remark,
		RealDelay:   time.Duration(grpcResult.LatencyMs) * time.Millisecond,
	}

	switch {
	case grpcResult.IsValid && grpcResult.Status == "SUCCESS":
		result.Status = core.CheckResultStatusSuccess
	case grpcResult.Status == "TIMEOUT":
		result.Status = core.CheckResultStatusUnavailable
		result.Error = "Connection timeout"
	default:
		result.Status = core.CheckResultStatusError
		result.Error = grpcResult.Error
	}

	return result
}

type GRPCCoreStats struct {
	TotalRequests  int64
	ActiveRequests int32
	RemoteStats    *CheckerStats
}
