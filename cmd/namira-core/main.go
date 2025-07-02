package main

import (
	"fmt"
	"log"
	"runtime"
	"time"

	"github.com/NamiraNet/namira-core/internal/config"
	"github.com/NamiraNet/namira-core/internal/core"
	"github.com/NamiraNet/namira-core/internal/github"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"
)

// Build-time variables (injected via -ldflags)
var (
	version   = "dev"     // Default for development
	commit    = "unknown" // Git commit hash
	date      = "unknown" // Build date
	goVersion = runtime.Version()
	platform  = runtime.GOOS + "/" + runtime.GOARCH

	port          string
	timeout       time.Duration
	maxConcurrent int
	checkHost     string

	cfg         *config.Config
	updater     *github.Updater
	redisClient *redis.Client
)

func getVersionInfo() string {
	commitHash := commit
	if len(commit) > 8 {
		commitHash = commit[:8]
	}
	return fmt.Sprintf("namira-core %s (%s) built with %s on %s at %s",
		version, commitHash, goVersion, platform, date)
}

var rootCmd = &cobra.Command{
	Use:     "namira-core",
	Version: version,
	Short:   "namira-core VPN Link Checker Service",
	Long:    `A service to check and validate various VPN protocol links including Vmess, Vless, Shadowsocks, and Trojan.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		fmt.Println(getVersionInfo())
	},
}

func init() {
	// Load configuration from environment variables
	cfg = config.Load()

	rootCmd.PersistentFlags().StringVarP(&port, "port", "p", "", "Port to run the service on")
	rootCmd.PersistentFlags().DurationVarP(&timeout, "timeout", "t", core.DefaultCheckTimeout, "Connection timeout")
	rootCmd.PersistentFlags().IntVarP(&maxConcurrent, "concurrent", "c", 0, "Maximum concurrent connections")
	rootCmd.PersistentFlags().StringVarP(&checkHost, "host", "H", "", "Host to check")
	rootCmd.SetVersionTemplate(getVersionInfo() + "\n")

	// Set default values if not provided
	if cfg.Server.Port == "" {
		cfg.Server.Port = port
	}
	if cfg.App.Timeout == 0 {
		cfg.App.Timeout = timeout
	}
	if cfg.App.MaxConcurrent == 0 {
		cfg.App.MaxConcurrent = maxConcurrent
	}
	if cfg.App.CheckHost == "" {
		cfg.App.CheckHost = checkHost
	}

	rootCmd.AddCommand(apiCmd, checkCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
