package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/NamiraNet/namira-core/internal/config"
	"github.com/NamiraNet/namira-core/internal/core"
	"go.uber.org/zap"
)

type GRPCCore struct {
	clients        []*CheckerClient
	clientTags     []string
	logger         *zap.Logger
	timeout        time.Duration
	maxConcurrent  int
	aggregateMode  bool // If true, send configs to all workers; if false, distribute efficiently
	totalRequests  atomic.Int64
	activeRequests atomic.Int32
	balanceIndex   atomic.Uint64
	mu             sync.RWMutex
}

type GRPCCoreOpts struct {
	CheckerServiceAddr string // Deprecated: use CheckerNodes instead
	CheckerNodes       []config.CheckerNodeConfig
	Timeout            time.Duration
	MaxConcurrent      int
	AggregateMode      bool // If true, send configs to all workers; if false, distribute efficiently
	Logger             *zap.Logger
	APIKey             string
	TLSConfig          *tls.Config
}

type GRPCCoreStats struct {
	TotalRequests  int64
	ActiveRequests int32
	RemoteStats    *CheckerStats
}

func NewGRPCCore(opts *GRPCCoreOpts) (*GRPCCore, error) {
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

	var clients []*CheckerClient
	var tags []string

	clientOpts := &CheckerClientOpts{
		APIKey:    opts.APIKey,
		TLSConfig: opts.TLSConfig,
	}
	// If using new multi-node configuration
	if len(opts.CheckerNodes) > 0 {
		for _, node := range opts.CheckerNodes {
			client, err := NewCheckerClient(node.Addr, opts.Logger, clientOpts)
			if err != nil {
				opts.Logger.Error("Failed to create checker client",
					zap.String("addr", node.Addr),
					zap.String("tag", node.Tag),
					zap.Error(err))
				continue
			}
			clients = append(clients, client)
			tags = append(tags, node.Tag)
			opts.Logger.Info("Connected to checker node",
				zap.String("addr", node.Addr),
				zap.String("tag", node.Tag))
		}
	} else if opts.CheckerServiceAddr != "" {
		// Fallback to legacy single node configuration
		client, err := NewCheckerClient(opts.CheckerServiceAddr, opts.Logger, clientOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to create checker client: %w", err)
		}
		clients = append(clients, client)
		tags = append(tags, "legacy")
	}

	if len(clients) == 0 {
		return nil, fmt.Errorf("no checker nodes available")
	}

	return &GRPCCore{
		clients:       clients,
		clientTags:    tags,
		logger:        opts.Logger,
		timeout:       opts.Timeout,
		maxConcurrent: opts.MaxConcurrent,
		aggregateMode: opts.AggregateMode,
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

	if g.aggregateMode {
		g.logger.Info("Starting comprehensive gRPC config check (all workers process all configs)",
			zap.String("job_id", jobID),
			zap.Int("config_count", len(configs)),
			zap.Int("worker_nodes", len(g.clients)))

		// Send each config to all workers for redundancy and combined results - PARALLEL VERSION
		g.processConfigsWithAllWorkers(ctx, jobID, configs, resultChan)
	} else {
		g.logger.Info("Starting efficient distributed gRPC config check",
			zap.String("job_id", jobID),
			zap.Int("config_count", len(configs)),
			zap.Int("worker_nodes", len(g.clients)))

		// Use efficient distribution (each config to one worker)
		g.distributeConfigsAcrossWorkers(ctx, jobID, configs, resultChan)
	}
}

func (g *GRPCCore) CheckConfigsList(configs []string) []core.CheckResult {
	results := make([]core.CheckResult, 0, len(configs))

	for result := range g.CheckConfigs(configs) {
		results = append(results, result)
	}

	return results
}

func (g *GRPCCore) HealthCheck(ctx context.Context) error {
	// Check health of all clients
	g.mu.RLock()
	clients := make([]*CheckerClient, len(g.clients))
	tags := make([]string, len(g.clientTags))
	copy(clients, g.clients)
	copy(tags, g.clientTags)
	g.mu.RUnlock()

	var lastErr error
	for i, client := range clients {
		if err := client.HealthCheck(ctx); err != nil {
			g.logger.Error("Health check failed for checker node",
				zap.String("addr", client.serverAddr),
				zap.String("tag", tags[i]),
				zap.Error(err))
			lastErr = err
		}
	}

	return lastErr
}

func (g *GRPCCore) GetStats(ctx context.Context) (*GRPCCoreStats, error) {
	// Get stats from the first available client (for backward compatibility)
	client := g.selectClient()
	if client == nil {
		return nil, fmt.Errorf("no available checker clients")
	}

	stats, err := client.GetStats(ctx)
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
	g.mu.Lock()
	defer g.mu.Unlock()

	var lastErr error
	for i, client := range g.clients {
		if err := client.Close(); err != nil {
			g.logger.Error("Failed to close checker client",
				zap.String("tag", g.clientTags[i]),
				zap.Error(err))
			lastErr = err
		}
	}

	g.clients = nil
	g.clientTags = nil
	return lastErr
}

// convertToCheckResult converts a gRPC response to a core CheckResult
func (g *GRPCCore) convertToCheckResult(grpcResult *CheckerResponse) core.CheckResult {
	result := core.CheckResult{
		Raw:            grpcResult.Config,
		Protocol:       grpcResult.Protocol,
		Server:         grpcResult.Server,
		CountryCode:    grpcResult.CountryCode,
		Remark:         grpcResult.Remark,
		RealDelay:      time.Duration(grpcResult.LatencyMs) * time.Millisecond,
		CheckerNodeTag: []string{grpcResult.CheckerNodeTag},
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

// selectClient returns a checker client using round-robin load balancing
func (g *GRPCCore) selectClient() *CheckerClient {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if len(g.clients) == 0 {
		return nil
	}

	index := g.balanceIndex.Add(1) % uint64(len(g.clients))
	return g.clients[index]
}

// Helper function for min (since it might not be available in older Go versions)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
