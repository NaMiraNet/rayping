package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/joho/godotenv/autoload"
)

const (
	defaultCheckerAddr = "localhost:50051"
)

// Config holds the base configuration
type Config struct {
	Server   ServerConfig
	Worker   WorkerConfig
	Redis    RedisConfig
	App      AppConfig
	Github   GithubConfig
	Telegram TelegramConfig
	GRPC     GRPCConfig
}

type ServerConfig struct {
	Port         string
	Host         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

type WorkerConfig struct {
	Count     int
	QueueSize int
}

type RedisConfig struct {
	Addr      string
	Password  string
	DB        int
	ResultTTL time.Duration
}

type GithubConfig struct {
	SSHKeyPath string
	Owner      string
	Repo       string
}

type AppConfig struct {
	LogLevel        string
	Timeout         time.Duration
	RefreshInterval time.Duration
	MaxConcurrent   int
	CheckHost       string
	EncryptionKey   string
}

type TelegramConfig struct {
	BotToken        string
	Channel         string
	Template        string
	QRConfig        string
	ProxyURL        string
	SendingInterval time.Duration
}

type GRPCConfig struct {
	CheckerServiceAddr string // Deprecated: use CheckerNodes instead
	CheckerNodes       []CheckerNodeConfig
	Timeout            time.Duration
	MaxConcurrent      int
	AggregateMode      bool // If true, send each config to all workers for redundancy; if false, distribute efficiently
	APIKey             string
	TLS                GRPCTLSConfig
}

type GRPCTLSConfig struct {
	CertFile string
	KeyFile  string
	CAFile   string
}

type CheckerNodeConfig struct {
	Addr string
	Tag  string
}

// Load loads configuration from environment variables with defaults value
func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         getEnv("SERVER_PORT", "8080"),
			Host:         getEnv("SERVER_HOST", ""),
			ReadTimeout:  getEnvDuration("SERVER_READ_TIMEOUT", 30*time.Second),
			WriteTimeout: getEnvDuration("SERVER_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:  getEnvDuration("SERVER_IDLE_TIMEOUT", 60*time.Second),
		},
		Worker: WorkerConfig{
			Count:     getEnvInt("WORKER_COUNT", 5),
			QueueSize: getEnvInt("WORKER_QUEUE_SIZE", 100),
		},
		Redis: RedisConfig{
			Addr:      getEnv("REDIS_ADDR", "localhost:6379"),
			Password:  getEnv("REDIS_PASSWORD", ""),
			DB:        getEnvInt("REDIS_DB", 0),
			ResultTTL: getEnvDuration("REDIS_RESULT_TTL", time.Hour),
		},
		Github: GithubConfig{
			SSHKeyPath: getEnv("GITHUB_SSH_KEY_PATH", ""),
			Owner:      getEnv("GITHUB_OWNER", ""),
			Repo:       getEnv("GITHUB_REPO", ""),
		},
		App: AppConfig{
			LogLevel:        getEnv("LOG_LEVEL", "info"),
			Timeout:         getEnvDuration("APP_TIMEOUT", 10*time.Second),
			MaxConcurrent:   getEnvInt("MAX_CONCURRENT", 0),
			CheckHost:       getEnv("CHECK_HOST", "1.1.1.1:80"),
			EncryptionKey:   getEnv("ENCRYPTION_KEY", ""),
			RefreshInterval: getEnvDuration("REFRESH_INTERVAL", time.Hour),
		},
		Telegram: TelegramConfig{
			BotToken:        getEnv("TELEGRAM_BOT_TOKEN", ""),
			Channel:         getEnv("TELEGRAM_CHANNEL", ""),
			Template:        getEnv("TELEGRAM_TEMPLATE", ""),
			QRConfig:        getEnv("TELEGRAM_QR_CONFIG", ""),
			ProxyURL:        getEnv("TELEGRAM_PROXY_URL", ""),
			SendingInterval: getEnvDuration("TELEGRAM_SENDING_INTERVAL", 10*time.Second),
		},
		GRPC: GRPCConfig{
			CheckerServiceAddr: getEnv("GRPC_CHECKER_SERVICE_ADDR", defaultCheckerAddr),
			CheckerNodes:       parseCheckerNodes(),
			Timeout:            getEnvDuration("GRPC_TIMEOUT", 5*time.Minute),
			MaxConcurrent:      getEnvInt("GRPC_MAX_CONCURRENT", 0),
			AggregateMode:      getEnvBool("GRPC_AGGREGATE_MODE", false), // Default to efficient distribution
			APIKey:             getEnv("GRPC_API_KEY", ""),
			TLS: GRPCTLSConfig{
				CertFile: getEnv("GRPC_TLS_CERT_FILE", ""),
				KeyFile:  getEnv("GRPC_TLS_KEY_FILE", ""),
				CAFile:   getEnv("GRPC_TLS_CA_FILE", ""),
			},
		},
	}
}

// Helper functions to get environment variables with defaults
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

// parseCheckerNodes parses checker nodes from environment variables
// Format: GRPC_CHECKER_NODES="addr1:tag1,addr2:tag2,addr3:tag3"
// Example: GRPC_CHECKER_NODES="localhost:50051:node1,localhost:50052:node2"
func parseCheckerNodes() []CheckerNodeConfig {
	nodesEnv := getEnv("GRPC_CHECKER_NODES", "")
	if nodesEnv == "" {
		// Fallback to single node configuration for backward compatibility
		addr := getEnv("GRPC_CHECKER_SERVICE_ADDR", defaultCheckerAddr)
		tag := getEnv("GRPC_CHECKER_NODE_TAG", "default")
		return []CheckerNodeConfig{
			{
				Addr: addr,
				Tag:  tag,
			},
		}
	}

	var nodes []CheckerNodeConfig
	pairs := strings.Split(nodesEnv, ",")
	for _, pair := range pairs {
		parts := strings.Split(strings.TrimSpace(pair), ":")
		if len(parts) >= 2 {
			addr := strings.Join(parts[:len(parts)-1], ":")
			tag := parts[len(parts)-1]
			nodes = append(nodes, CheckerNodeConfig{
				Addr: addr,
				Tag:  tag,
			})
		}
	}

	// If no valid nodes parsed, fallback to default
	if len(nodes) == 0 {
		addr := getEnv("GRPC_CHECKER_SERVICE_ADDR", defaultCheckerAddr)
		tag := getEnv("GRPC_CHECKER_NODE_TAG", "default")
		return []CheckerNodeConfig{
			{
				Addr: addr,
				Tag:  tag,
			},
		}
	}

	return nodes
}
