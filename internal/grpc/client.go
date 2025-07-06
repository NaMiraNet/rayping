package grpc

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	checkerpb "github.com/NamiraNet/namira-core/proto"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
)

const (
	defaultTimeout    = 30 * time.Minute
	defaultBufferSize = 10 * 1024 * 1024 // 10MB
	keepAliveTime     = 30 * time.Second
	keepAliveTimeout  = 3 * time.Second
	configCheckBuffer = 2
)

type CheckerClient struct {
	conn       *grpc.ClientConn
	client     checkerpb.ConfigCheckerClient
	logger     *zap.Logger
	serverAddr string
	timeout    time.Duration

	// Connection management
	mu           sync.RWMutex
	connected    bool
	reconnecting bool
}

type CheckerResponse struct {
	JobID       string
	Config      string
	IsValid     bool
	LatencyMs   int64
	Error       string
	Protocol    string
	Server      string
	CountryCode string
	Remark      string
	Status      string
	Timestamp   time.Time
}

type CheckerStats struct {
	TotalChecks      int64
	SuccessfulChecks int64
	FailedChecks     int64
	SuccessRate      float64
	UptimeSeconds    int64
}

func NewCheckerClient(serverAddr string, logger *zap.Logger) (*CheckerClient, error) {
	client := &CheckerClient{
		serverAddr: serverAddr,
		logger:     logger,
		timeout:    defaultTimeout,
	}
	return client, client.connect()
}

func (c *CheckerClient) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	conn, err := c.createConnection()
	if err != nil {
		return fmt.Errorf("failed to connect to checker service: %w", err)
	}

	c.conn = conn
	c.client = checkerpb.NewConfigCheckerClient(conn)
	c.connected = true
	c.logger.Info("Connected to checker service", zap.String("addr", c.serverAddr))
	return nil
}

func (c *CheckerClient) createConnection() (*grpc.ClientConn, error) {
	return grpc.NewClient(c.serverAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                keepAliveTime,
			Timeout:             keepAliveTimeout,
			PermitWithoutStream: true,
		}),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(defaultBufferSize),
			grpc.MaxCallSendMsgSize(defaultBufferSize),
		),
	)
}

func (c *CheckerClient) CheckConfigs(ctx context.Context, jobID string, configs []string) (<-chan *CheckerResponse, error) {
	if err := c.ensureConnected(); err != nil {
		return nil, err
	}

	stream, err := c.client.CheckConfigs(ctx)
	if err != nil {
		c.logger.Error("Failed to create check stream", zap.Error(err))
		return nil, err
	}

	resultChan := make(chan *CheckerResponse, len(configs)*configCheckBuffer)
	go c.sendConfigs(ctx, stream, jobID, configs)
	go c.receiveResponses(ctx, stream, jobID, resultChan)

	return resultChan, nil
}

func (c *CheckerClient) sendConfigs(ctx context.Context, stream checkerpb.ConfigChecker_CheckConfigsClient, jobID string, configs []string) {
	defer func() {
		if err := stream.CloseSend(); err != nil {
			c.logger.Error("Failed to close check stream", zap.Error(err))
		}
	}()

	for i, config := range configs {
		select {
		case <-ctx.Done():
			c.logger.Info("Context canceled, stopping sending configs", zap.String("job_id", jobID))
			return
		default:
			req := &checkerpb.CheckRequest{
				JobId:          jobID,
				Config:         config,
				TimeoutSeconds: 10,
				Metadata: map[string]string{
					"index": fmt.Sprintf("%d", i),
					"total": fmt.Sprintf("%d", len(configs)),
				},
			}

			if err := stream.Send(req); err != nil {
				c.logger.Error("Failed to send request", zap.Error(err), zap.String("job_id", jobID), zap.Int("config_index", i))
				return
			}

			c.logger.Debug("Sent config for checking",
				zap.String("job_id", jobID),
				zap.Int("index", i),
				zap.String("config", config[:min(50, len(config))]))
		}
	}

	c.logger.Info("Sent all configs for checking", zap.String("job_id", jobID), zap.Int("total_configs", len(configs)))
}

func (c *CheckerClient) receiveResponses(ctx context.Context, stream checkerpb.ConfigChecker_CheckConfigsClient, jobID string, resultChan chan<- *CheckerResponse) {
	defer close(resultChan)

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			c.logger.Debug("Stream ended normally", zap.String("job_id", jobID))
			return
		}
		if err != nil {
			c.logStreamError(jobID, err)
			return
		}

		result := &CheckerResponse{
			JobID:       resp.JobId,
			Config:      resp.Config,
			IsValid:     resp.IsValid,
			LatencyMs:   resp.LatencyMs,
			Error:       resp.ErrorMessage,
			Protocol:    resp.Protocol,
			Server:      resp.Server,
			CountryCode: resp.CountryCode,
			Remark:      resp.Remark,
			Status:      resp.Status.String(),
			Timestamp:   resp.Timestamp.AsTime(),
		}

		select {
		case resultChan <- result:
		case <-ctx.Done():
			c.logger.Info("Context cancelled, stopping result processing", zap.String("job_id", jobID))
			return
		}
	}
}

func (c *CheckerClient) logStreamError(jobID string, err error) {
	if st, ok := status.FromError(err); ok {
		c.logger.Error("Stream error",
			zap.String("job_id", jobID),
			zap.String("code", st.Code().String()),
			zap.String("message", st.Message()))
	} else {
		c.logger.Error("Failed to receive response",
			zap.String("job_id", jobID),
			zap.Error(err))
	}
}

func (c *CheckerClient) HealthCheck(ctx context.Context) error {
	if err := c.ensureConnected(); err != nil {
		return err
	}

	_, err := c.client.Health(ctx, &checkerpb.HealthRequest{})
	return err
}

func (c *CheckerClient) GetStats(ctx context.Context) (*CheckerStats, error) {
	if err := c.ensureConnected(); err != nil {
		return nil, err
	}

	resp, err := c.client.GetStats(ctx, &checkerpb.StatsRequest{})
	if err != nil {
		return nil, err
	}

	return &CheckerStats{
		TotalChecks:      resp.TotalChecks,
		SuccessfulChecks: resp.SuccessfulChecks,
		FailedChecks:     resp.FailedChecks,
		SuccessRate:      resp.SuccessRate,
		UptimeSeconds:    resp.UptimeSeconds,
	}, nil
}

func (c *CheckerClient) ensureConnected() error {
	c.mu.RLock()
	if c.connected {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()

	return c.reconnect()
}

func (c *CheckerClient) reconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.reconnecting {
		return fmt.Errorf("already reconnecting")
	}

	c.reconnecting = true
	defer func() { c.reconnecting = false }()

	if c.conn != nil {
		c.conn.Close()
		c.connected = false
	}

	c.logger.Info("Reconnecting to checker service", zap.String("addr", c.serverAddr))

	conn, err := c.createConnection()
	if err != nil {
		return fmt.Errorf("failed to reconnect to checker service: %w", err)
	}

	c.conn = conn
	c.client = checkerpb.NewConfigCheckerClient(conn)
	c.connected = true

	c.logger.Info("Successfully reconnected to checker service", zap.String("addr", c.serverAddr))
	return nil
}

func (c *CheckerClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		err := c.conn.Close()
		c.connected = false
		c.logger.Info("Closed connection to checker service")
		return err
	}

	return nil
}
