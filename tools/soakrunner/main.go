package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/dsn/dsn/tools/soakrunner/pkg/harness"
	"github.com/spf13/cobra"
)

var (
	cfgPath  string
	profile  string
	duration time.Duration
	dryRun   bool
	logLevel string
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "soakrunner",
	Short: "Soak testing tool for DSN blockchain",
	Long:  `A tool that generates and submits transactions for soak testing`,
	RunE:  run,
}

func init() {
	rootCmd.Flags().StringVar(&cfgPath, "config", "config.yaml", "Path to config file")
	rootCmd.Flags().StringVar(&profile, "profile", "moderate", "Workload profile: light, moderate, heavy, burst, wasm, mixed")
	rootCmd.Flags().DurationVar(&duration, "duration", 1*time.Hour, "Duration to run the test")
	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print what would happen without connecting")
	rootCmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level: debug, info, warn, error")
}

func run(cmd *cobra.Command, args []string) error {
	// Setup logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(logLevel),
	}))

	// Load config
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Override config with CLI flags
	if cmd.Flags().Changed("profile") {
		cfg.Profile = profile
	}
	if cmd.Flags().Changed("duration") {
		cfg.Duration = duration
	}
	if cmd.Flags().Changed("dry-run") {
		cfg.DryRun = dryRun
	}

	// Validate config
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	logger.Info("Configuration loaded",
		"endpoint", cfg.NodeEndpoint,
		"profile", cfg.Profile,
		"duration", cfg.Duration,
		"tps", cfg.TPS,
		"dryRun", cfg.DryRun,
	)

	// Create harness
	harness, err := newHarness(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create harness: %w", err)
	}

	// Run the test
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration+time.Minute)
	defer cancel()

	if err := harness.Run(ctx); err != nil && err != context.Canceled {
		return fmt.Errorf("harness failed: %w", err)
	}

	// Print summary
	harness.PrintSummary()

	return nil
}

func newHarness(cfg *Config, logger *slog.Logger) (*harness.Harness, error) {
	h := harness.HarnessConfig{
		NodeEndpoint: cfg.NodeEndpoint,
		Duration:     cfg.Duration,
		TPS:          cfg.TPS,
		Profile:      cfg.Profile,
		LogInterval:  cfg.LogInterval,
		DryRun:       cfg.DryRun,
	}

	return harness.NewHarness(h, logger)
}

func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
