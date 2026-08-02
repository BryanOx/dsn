package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/dsn/dsn/config"
	"github.com/spf13/cobra"
)

// ConfigFlags holds command-line flags for config overrides.
// These take highest precedence over config file and env vars.
type ConfigFlags struct {
	configPath string

	// P2P
	p2pPort        int
	p2pMaxPeers    int
	bootstrapPeers string

	// RPC
	rpcPort int

	// Metrics
	metricsPort int

	// Storage
	dataDir string

	// Chain
	chainID     uint32
	mempoolSize int
	indexer     bool
	fastSync    bool

	// Logging
	logLevel string
}

var cfgFlags ConfigFlags

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "dsn",
	Short: "DSN - Deterministic Settlement Network CLI",
	Long: `DSN CLI provides commands for interacting with the DSN blockchain.

Node Management:
  node       Start a production node with P2P, RPC, and consensus
  devnet     Start a local devnet node with RPC, indexer, and explorer

Genesis & Network:
  genesis    Genesis file operations (init, validate, devnet)

Validator Operations:
  validator  Validator operations (init, register, status)

Wallet & Contracts:
  wallet     Wallet management (generate, sign, nonce)
  contract   Smart contract operations (deploy, call, estimate)

Examples:
  dsn devnet                           # Start local devnet
  dsn genesis init --devnet            # Create devnet genesis
  dsn validator init                   # Generate validator key
  dsn wallet generate                  # Generate wallet key

For more information, see the documentation at https://docs.dsn.io`,
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Add global flags here if needed
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "quiet output (only errors)")

	// Config file path (lowest precedence)
	rootCmd.PersistentFlags().StringVar(&cfgFlags.configPath, "config", "", "path to config file (TOML)")

	// P2P config flags
	rootCmd.PersistentFlags().IntVar(&cfgFlags.p2pPort, "p2p-port", 0, "P2P port (0 = disabled)")
	rootCmd.PersistentFlags().IntVar(&cfgFlags.p2pMaxPeers, "p2p-max-peers", 0, "maximum P2P peers")
	rootCmd.PersistentFlags().StringVar(&cfgFlags.bootstrapPeers, "bootstrap-peers", "", "comma-separated bootstrap peer addresses")

	// RPC config flag
	rootCmd.PersistentFlags().IntVar(&cfgFlags.rpcPort, "rpc-port", 0, "RPC server port")

	// Metrics config flag
	rootCmd.PersistentFlags().IntVar(&cfgFlags.metricsPort, "metrics-port", 0, "Prometheus metrics port")

	// Storage config flag
	rootCmd.PersistentFlags().StringVar(&cfgFlags.dataDir, "data-dir", "", "data directory for node storage")

	// Chain config flags
	rootCmd.PersistentFlags().Uint32Var(&cfgFlags.chainID, "chain-id", 0, "chain ID")
	rootCmd.PersistentFlags().IntVar(&cfgFlags.mempoolSize, "mempool-size", 0, "mempool max size")
	rootCmd.PersistentFlags().BoolVar(&cfgFlags.indexer, "indexer", false, "enable block indexer")
	rootCmd.PersistentFlags().BoolVar(&cfgFlags.fastSync, "fast-sync", false, "enable fast sync mode")

	// Logging config flag
	rootCmd.PersistentFlags().StringVar(&cfgFlags.logLevel, "log-level", "", "log level (debug, info, warn, error)")

	// Add command groups
	// Node commands
	rootCmd.AddCommand(nodeCmd)

	// Genesis commands
	rootCmd.AddCommand(genesisCmd)
	genesisCmd.AddCommand(genesisInitCmd)
	genesisCmd.AddCommand(genesisValidateCmd)

	// Validator commands
	rootCmd.AddCommand(validatorCmd)
	validatorCmd.AddCommand(validatorInitCmd)
	validatorCmd.AddCommand(validatorRegisterCmd)
	validatorCmd.AddCommand(validatorStatusCmd)

	// Keep existing commands (devnet, wallet, contract)
	// These are added in their respective files via init()
}

// BuildConfig builds a Config from all sources: defaults, config file, env vars, CLI flags.
// Priority: CLI flags > env vars > config file > defaults
func BuildConfig() *config.Config {
	defaults := config.DefaultConfig()
	cfg := &defaults

	// Load from config file if specified
	if cfgFlags.configPath != "" {
		if fileCfg, err := config.LoadConfig(cfgFlags.configPath); err == nil {
			cfg = fileCfg
		}
	}

	// Override with environment variables
	cfg = config.LoadConfigFromEnv(cfg)

	// Override with CLI flags (highest precedence)
	applyCLIFlags(cfg)

	return cfg
}

// applyCLIFlags applies CLI flag overrides to the config.
func applyCLIFlags(cfg *config.Config) {
	// P2P flags
	if cfgFlags.p2pPort != 0 {
		cfg.P2P.Port = cfgFlags.p2pPort
	}
	if cfgFlags.p2pMaxPeers != 0 {
		cfg.P2P.MaxPeers = cfgFlags.p2pMaxPeers
	}
	if cfgFlags.bootstrapPeers != "" {
		cfg.P2P.BootstrapPeers = parseBootstrapPeersFromString(cfgFlags.bootstrapPeers)
	}

	// RPC flag
	if cfgFlags.rpcPort != 0 {
		cfg.RPC.Port = cfgFlags.rpcPort
	}

	// Metrics flag
	if cfgFlags.metricsPort != 0 {
		cfg.Metrics.Port = cfgFlags.metricsPort
	}

	// Storage flag
	if cfgFlags.dataDir != "" {
		cfg.Storage.DataDir = cfgFlags.dataDir
	}

	// Chain flags
	if cfgFlags.chainID != 0 {
		cfg.Chain.ChainID = cfgFlags.chainID
	}
	if cfgFlags.mempoolSize != 0 {
		cfg.Chain.MempoolMaxSize = cfgFlags.mempoolSize
	}
	cfg.Chain.IndexerEnabled = cfgFlags.indexer
	cfg.Chain.FastSyncEnabled = cfgFlags.fastSync

	// Logging flag
	if cfgFlags.logLevel != "" {
		cfg.Logging.Level = cfgFlags.logLevel
	}
}

// parseBootstrapPeersFromString parses a comma-separated list of peer addresses.
func parseBootstrapPeersFromString(s string) []string {
	if s == "" {
		return nil
	}
	var peers []string
	for _, p := range strings.Split(s, ",") {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			peers = append(peers, trimmed)
		}
	}
	return peers
}

// printBanner prints the DSN banner
func printBanner() {
	fmt.Println("╔══════════════════════════════════════════╗")
	fmt.Println("║  DSN — Deterministic Settlement Network  ║")
	fmt.Println("║  Version 0.1                             ║")
	fmt.Println("╚══════════════════════════════════════════╝")
}

// fatal prints error and exits
func fatal(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
