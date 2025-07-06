package config

import (
	"os"
	"strconv"
	"time"

	_ "github.com/joho/godotenv/autoload"
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
	CheckerServiceAddr string
	Timeout            time.Duration
	MaxConcurrent      int
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
			CheckerServiceAddr: getEnv("GRPC_CHECKER_SERVICE_ADDR", "localhost:50051"),
			Timeout:            getEnvDuration("GRPC_TIMEOUT", 5*time.Minute),
			MaxConcurrent:      getEnvInt("GRPC_MAX_CONCURRENT", 0),
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
