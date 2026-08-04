package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/BryanOx/dsn/config"
	"github.com/BryanOx/dsn/node"
	"github.com/BryanOx/dsn/rpc"
	"github.com/BryanOx/dsn/rpc/service"
	"github.com/spf13/cobra"
)

// NodeFlags holds flags for the node start command.
type NodeFlags struct {
	genesisFile  string
	validatorKey string
	dataDir      string
}

var nodeFlags NodeFlags

// nodeCmd represents the node command
var nodeCmd = &cobra.Command{
	Use:   "node",
	Short: "Start a production DSN node",
	Long: `Start a production node with full P2P networking, RPC server,
and validator capabilities.

Examples:
  dsn node start
  dsn node start --config node.toml
  dsn node start --genesis genesis.json --p2p-port 30303 --rpc-port 8545`,
}

var nodeStartCmd = &cobra.Command{
	Use:   "start [flags]",
	Short: "Start the production node",
	Long: `Start the production node with the loaded configuration.

The node will:
- Load and validate configuration
- Load and validate genesis document
- Initialize persistent storage (if DataDir configured)
- Initialize genesis state (if fresh DB)
- Load validator registry
- Start P2P networking (if enabled)
- Start RPC server (if enabled)
- Start metrics endpoint (if enabled)
- Enter sync mode and start consensus participation

Examples:
  dsn node start
  dsn node start --config node.toml
  dsn node start --genesis genesis.json --data-dir ./data`,
	RunE: runNodeStart,
}

func init() {
	nodeCmd.AddCommand(nodeStartCmd)

	// Add flags for node start
	nodeStartCmd.Flags().StringVar(&nodeFlags.genesisFile, "genesis", "", "path to genesis file (overrides config)")
	nodeStartCmd.Flags().StringVar(&nodeFlags.validatorKey, "validator-key", "", "path to validator key file")
	nodeStartCmd.Flags().StringVar(&nodeFlags.dataDir, "data-dir", "", "data directory for node storage (overrides config)")
}

func runNodeStart(cmd *cobra.Command, args []string) error {
	printBanner()

	// Step 1: Load config from file, env vars, and CLI flags
	// Priority: CLI flags > env vars > config file > defaults
	cfg := BuildConfig()

	// Override with CLI flags (highest precedence)
	if nodeFlags.genesisFile != "" {
		cfg.Genesis.File = nodeFlags.genesisFile
	}
	if nodeFlags.dataDir != "" {
		cfg.Storage.DataDir = nodeFlags.dataDir
	}
	if nodeFlags.validatorKey != "" {
		cfg.Validator.KeyFile = nodeFlags.validatorKey
	}

	// Validate config
	fmt.Println("=== Config ===")
	if err := config.ValidateConfig(cfg); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}
	fmt.Println("Config: OK")

	// Check that genesis file is specified
	if cfg.Genesis.File == "" {
		return fmt.Errorf("genesis file is required (use --genesis flag or config genesis.file)\nRun 'dsn genesis init --devnet' to create a development genesis file")
	}

	// Verify genesis file exists
	fmt.Println("=== Genesis ===")
	if _, err := os.Stat(cfg.Genesis.File); os.IsNotExist(err) {
		return fmt.Errorf("genesis file not found at '%s'\nRun 'dsn genesis init --devnet' to create a development genesis file", cfg.Genesis.File)
	}
	fmt.Printf("Genesis file: %s\n", cfg.Genesis.File)

	// Step 2: Convert config.Config to node.Config
	nodeCfg := convertToNodeConfig(cfg)

	// Step 3: Create node
	fmt.Println("Initializing node...")
	n, err := node.New(nodeCfg)
	if err != nil {
		return fmt.Errorf("failed to create node: %w", err)
	}

	// Step 4: Start node with full lifecycle
	fmt.Println("Starting node...")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := n.Start(ctx); err != nil {
		return fmt.Errorf("node start failed: %w", err)
	}

	// Start JSON-RPC server if enabled.
	// rpc.Server exposes no shutdown; its lifetime is tied to the process.
	if cfg.RPC.Enabled && cfg.RPC.Port > 0 {
		svc := service.NewNodeService(n)
		rpcServer := rpc.NewWithService(svc)
		go func() {
			fmt.Printf("RPC server listening on http://localhost:%d\n", cfg.RPC.Port)
			if err := rpcServer.Serve(fmt.Sprintf(":%d", cfg.RPC.Port)); err != nil {
				log.Printf("RPC server error: %v", err)
			}
		}()
	}

	// Step 5: Wait for shutdown signal
	fmt.Println()
	fmt.Println("Node is running. Press Ctrl+C to stop.")
	fmt.Println()

	// Print useful info
	if n.P2P() != nil {
		fmt.Printf("P2P: /ip4/0.0.0.0/tcp/%d\n", cfg.P2P.Port)
	}
	if cfg.RPC.Enabled && cfg.RPC.Port > 0 {
		fmt.Printf("RPC: http://localhost:%d\n", cfg.RPC.Port)
	}
	if cfg.Metrics.Enabled && cfg.Metrics.Port > 0 {
		fmt.Printf("Metrics: http://localhost:%d/metrics\n", cfg.Metrics.Port)
	}
	fmt.Println()

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nReceived shutdown signal...")

	// Graceful shutdown
	if err := n.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}

	fmt.Println("Node stopped gracefully")
	return nil
}

// convertToNodeConfig converts config.Config to node.Config.
func convertToNodeConfig(cfg *config.Config) node.Config {
	return node.Config{
		ChainID:                 cfg.Chain.ChainID,
		DataDir:                 cfg.Storage.DataDir,
		MempoolMaxSize:          cfg.Chain.MempoolMaxSize,
		MempoolTTL:              cfg.Chain.MempoolTTL,
		RPCPort:                 cfg.RPC.Port,
		P2PPort:                 cfg.P2P.Port,
		Validators:              nil, // Will be loaded from DB
		MaxTxPerBlock:           cfg.Chain.MaxTxPerBlock,
		ProposerTimeout:         cfg.Chain.ProposerTimeout,
		GenesisFile:             cfg.Genesis.File,
		ValidatorKeyFile:        cfg.Validator.KeyFile,
		MaxPeers:                cfg.P2P.MaxPeers,
		MetricsPort:             cfg.Metrics.Port,
		IndexerEnabled:          cfg.Chain.IndexerEnabled,
		SnapshotInterval:        cfg.Snapshot.Interval,
		FastSyncEnabled:         cfg.Chain.FastSyncEnabled,
		TrustedCheckpointHeight: cfg.Chain.TrustedCheckpointHeight,
		TrustedCheckpointHash:   cfg.Chain.TrustedCheckpointHash,
		BootstrapPeers:          cfg.P2P.BootstrapPeers,
	}
}
