// Package config provides the hierarchical configuration system for DSN nodes.
package config

import (
	"os"
	"time"
)

// Config is the root configuration structure for DSN nodes.
// It contains all sub-configs for different components.
type Config struct {
	// P2P contains peer-to-peer networking configuration.
	P2P P2PConfig `toml:"p2p"`

	// RPC contains JSON-RPC server configuration.
	RPC RPCConfig `toml:"rpc"`

	// Metrics contains Prometheus metrics server configuration.
	Metrics MetricsConfig `toml:"metrics"`

	// Storage contains data persistence configuration.
	Storage StorageConfig `toml:"storage"`

	// Snapshot contains state snapshot configuration.
	Snapshot SnapshotConfig `toml:"snapshot"`

	// Validator contains validator-specific configuration.
	Validator ValidatorConfig `toml:"validator"`

	// Logging contains logging configuration.
	Logging LoggingConfig `toml:"logging"`

	// Chain contains chain-level configuration.
	Chain ChainConfig `toml:"chain"`

	// Genesis contains genesis file configuration.
	Genesis GenesisConfig `toml:"genesis"`
}

// P2PConfig contains peer-to-peer networking settings.
type P2PConfig struct {
	// ListenAddr is the IP address to listen on for P2P connections.
	// Default: "0.0.0.0"
	ListenAddr string `toml:"listen_addr"`

	// Port is the port number for P2P connections.
	// Must be in range 1024-65535.
	// Default: 0 (disabled)
	Port int `toml:"port"`

	// MaxPeers is the maximum number of connected peers.
	// Must be greater than 0.
	// Default: 50
	MaxPeers int `toml:"max_peers"`

	// BootstrapPeers is a list of peer addresses to connect to on startup.
	// Format: "/ip4/1.2.3.4/tcp/1234/p2p/PeerID"
	BootstrapPeers []string `toml:"bootstrap_peers"`

	// PingInterval is the interval between ping messages to peers.
	// Default: 30s
	PingInterval time.Duration `toml:"ping_interval"`
}

// RPCConfig contains JSON-RPC server settings.
type RPCConfig struct {
	// Enabled indicates whether the RPC server should be started.
	// Default: true
	Enabled bool `toml:"enabled"`

	// ListenAddr is the IP address to listen on for RPC connections.
	// Default: "0.0.0.0"
	ListenAddr string `toml:"listen_addr"`

	// Port is the port number for RPC server.
	// Must be in range 1024-65535.
	// Default: 8545
	Port int `toml:"port"`

	// CORSOrigins contains allowed CORS origins.
	// Empty means no CORS restrictions.
	CORSOrigins []string `toml:"cors_origins"`
}

// MetricsConfig contains Prometheus metrics server settings.
type MetricsConfig struct {
	// Enabled indicates whether the metrics server should be started.
	// Default: true
	Enabled bool `toml:"enabled"`

	// ListenAddr is the IP address to listen on for metrics endpoint.
	// Default: "0.0.0.0"
	ListenAddr string `toml:"listen_addr"`

	// Port is the port number for metrics server.
	// Must be in range 1024-65535.
	// Default: 9464
	Port int `toml:"port"`
}

// StorageConfig contains data persistence settings.
type StorageConfig struct {
	// DataDir is the directory for storing node data.
	// If empty, uses in-memory storage only.
	// Default: ""
	DataDir string `toml:"data_dir"`

	// MaxDBSize is the maximum size of the database in bytes.
	// Default: 10GB (10737418240)
	MaxDBSize int64 `toml:"max_db_size"`

	// FSync enables synchronous writes to BoltDB.
	// When true, every commit is fsynced to disk before returning.
	// This is slower but ensures durability across crashes.
	// When false (default), uses async writes for better performance.
	// Default: false
	FSync bool `toml:"fsync"`
}

// SnapshotConfig contains state snapshot settings.
type SnapshotConfig struct {
	// Enable indicates whether automatic snapshots are enabled.
	// Default: true
	Enable bool `toml:"enable"`

	// Interval is the number of epochs between snapshots.
	// Default: 10
	Interval uint64 `toml:"interval"`

	// MaxSnapshots is the maximum number of snapshots to retain.
	// Default: 5
	MaxSnapshots uint64 `toml:"max_snapshots"`

	// OutputDir is the directory to save snapshots.
	// Default: "<DataDir>/snapshots"
	OutputDir string `toml:"output_dir"`
}

// ValidatorConfig contains validator-specific configuration.
type ValidatorConfig struct {
	// KeyFile is the path to the validator key file.
	// Default: "validator_key.json"
	KeyFile string `toml:"key_file"`

	// Stake is the amount of DSN tokens staked.
	// Default: 0
	Stake uint64 `toml:"stake"`

	// CommissionRate is the validator commission rate (0-10000 = 0-100%).
	// Default: 1000 (10%)
	CommissionRate uint64 `toml:"commission_rate"`
}

// LoggingConfig contains logging configuration.
type LoggingConfig struct {
	// Level is the log level (debug, info, warn, error).
	// Default: "info"
	Level string `toml:"level"`

	// Format is the log format (text, json).
	// Default: "text"
	Format string `toml:"format"`

	// Output is the output target (stdout, stderr, file path).
	// Default: "stdout"
	Output string `toml:"output"`

	// EnableFileLogging enables logging to a file.
	// Default: false
	EnableFileLogging bool `toml:"enable_file_logging"`
}

// ChainConfig contains chain-level configuration.
type ChainConfig struct {
	// ChainID is the network chain ID.
	// Default: 0 (devnet)
	ChainID uint32 `toml:"chain_id"`

	// MempoolMaxSize is the maximum number of transactions in mempool.
	// Default: 10000
	MempoolMaxSize int `toml:"mempool_max_size"`

	// MempoolTTL is the time after which transactions expire from mempool.
	// Default: 5m
	MempoolTTL time.Duration `toml:"mempool_ttl"`

	// MaxTxPerBlock is the maximum number of transactions per block.
	// Default: 100
	MaxTxPerBlock int `toml:"max_tx_per_block"`

	// ProposerTimeout is the timeout for block proposal.
	// Default: 5s
	ProposerTimeout time.Duration `toml:"proposer_timeout"`

	// IndexerEnabled enables the block indexer.
	// Default: false
	IndexerEnabled bool `toml:"indexer_enabled"`

	// FastSyncEnabled enables fast sync mode at startup.
	// Default: false
	FastSyncEnabled bool `toml:"fast_sync_enabled"`

	// TrustedCheckpointHeight is the height of a known-good checkpoint.
	// Default: 0
	TrustedCheckpointHeight uint64 `toml:"trusted_checkpoint_height"`

	// TrustedCheckpointHash is the expected hash at checkpoint height.
	// Default: ""
	TrustedCheckpointHash string `toml:"trusted_checkpoint_hash"`
}

// GenesisConfig contains genesis file configuration.
type GenesisConfig struct {
	// File is the path to the genesis JSON file.
	// Default: ""
	File string `toml:"file"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		P2P: P2PConfig{
			ListenAddr:     "0.0.0.0",
			Port:           0, // 0 = disabled
			MaxPeers:       50,
			BootstrapPeers: []string{},
			PingInterval:   30 * time.Second,
		},
		RPC: RPCConfig{
			Enabled:     true,
			ListenAddr:  "0.0.0.0",
			Port:        8545,
			CORSOrigins: []string{},
		},
		Metrics: MetricsConfig{
			Enabled:    true,
			ListenAddr: "0.0.0.0",
			Port:       9464,
		},
		Storage: StorageConfig{
			DataDir:   "",
			MaxDBSize: 10 * 1024 * 1024 * 1024, // 10GB
		},
		Snapshot: SnapshotConfig{
			Enable:       true,
			Interval:     10,
			MaxSnapshots: 5,
			OutputDir:    "",
		},
		Validator: ValidatorConfig{
			KeyFile:        "validator_key.json",
			Stake:          0,
			CommissionRate: 1000, // 10%
		},
		Logging: LoggingConfig{
			Level:             "info",
			Format:            "text",
			Output:            "stdout",
			EnableFileLogging: false,
		},
		Chain: ChainConfig{
			ChainID:                 0,
			MempoolMaxSize:          10000,
			MempoolTTL:              5 * time.Minute,
			MaxTxPerBlock:           100,
			ProposerTimeout:         5 * time.Second,
			IndexerEnabled:          false,
			FastSyncEnabled:         true,
			TrustedCheckpointHeight: 0,
			TrustedCheckpointHash:   "",
		},
		Genesis: GenesisConfig{
			File: "",
		},
	}
}

// LoadConfig loads configuration from a TOML file.
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		return nil, ErrConfigPathRequired
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, ErrConfigFileNotFound
	}

	cfg := DefaultConfig()

	// Read file content
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, ErrConfigReadFailed.Wrap(err)
	}

	// Parse TOML
	_, err = tomlDecode(data, &cfg)
	if err != nil {
		return nil, ErrConfigParseFailed.Wrap(err)
	}

	return &cfg, nil
}

// tomlDecode is a wrapper to allow dependency injection for testing.
var tomlDecode = func(data []byte, cfg *Config) ([]string, error) {
	return Decode(data, cfg)
}
