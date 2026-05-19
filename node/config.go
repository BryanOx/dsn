package node

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dsn/dsn/types"
)

type Config struct {
	ChainID         uint32
	DataDir         string
	MempoolMaxSize  int
	MempoolTTL      time.Duration
	RPCPort         int
	P2PPort         int
	Validators      []types.Address
	MaxTxPerBlock   int
	ProposerTimeout time.Duration

	// GenesisFile is the path to the genesis JSON file.
	GenesisFile string

	// ValidatorKeyFile is the path to the validator key file.
	// If empty, node runs in read-only mode.
	ValidatorKeyFile string

	// MaxPeers is the maximum number of P2P peers.
	MaxPeers int

	// MetricsPort is the port for Prometheus metrics endpoint.
	// 0 = disabled.
	MetricsPort int

	// IndexerEnabled enables the block indexer.
	// Requires DataDir to be set.
	IndexerEnabled bool

	// SnapshotInterval sets how often (in epochs) state snapshots are taken.
	// 0 = no automatic snapshots at epoch boundaries.
	// E.g. 1 = snapshot every epoch, 10 = every 10 epochs.
	SnapshotInterval uint64

	// FastSyncEnabled enables fast sync mode at startup.
	// When true, the node will attempt fast sync from a snapshot
	// instead of normal block-by-block consensus.
	FastSyncEnabled bool

	// TrustedCheckpointHeight is the height of a known-good checkpoint
	// for fast sync. Used with TrustedCheckpointHash for verification.
	// 0 means no trusted checkpoint configured.
	TrustedCheckpointHeight uint64

	// TrustedCheckpointHash is the hex-encoded snapshot hash expected
	// at TrustedCheckpointHeight. Empty means no verification.
	TrustedCheckpointHash string

	// BootstrapPeers is a list of "ip:port" addresses for initial peer discovery.
	BootstrapPeers []string

	// VMTimeoutSeconds is the execution timeout for WASM contracts in seconds.
	// Default is 30 seconds.
	VMTimeoutSeconds uint64

	// FSync enables synchronous writes to BoltDB.
	// When true, every commit is fsynced to disk before returning.
	// This is slower but ensures durability across crashes.
	// When false (default), uses async writes for better performance.
	FSync bool
}

func DefaultConfig() Config {
	return Config{
		ChainID:         0,
		DataDir:         "", // empty = in-memory only. Set to a path for persistent storage.
		MempoolMaxSize:  10000,
		MempoolTTL:      300 * time.Second,
		RPCPort:         8545,
		P2PPort:         0, // 0 = P2P disabled (use env var DSN_P2P_PORT to enable)
		Validators:      []types.Address{},
		MaxTxPerBlock:   100,
		ProposerTimeout:       5 * time.Second,
		GenesisFile:            "",    // genesis file path
		ValidatorKeyFile:       "",    // validator key file path
		MaxPeers:               50,     // max P2P peers
		MetricsPort:            9464,   // Prometheus metrics port
		IndexerEnabled:        false,  // disabled by default
		SnapshotInterval:       10,    // snapshot every 10 epochs
		FastSyncEnabled:        false,  // normal sync by default
		TrustedCheckpointHeight: 0,
		TrustedCheckpointHash:  "",
		VMTimeoutSeconds:       30,    // 30 second VM execution timeout
		FSync:                  false, // async writes by default
	}
}

// ConfigFromEnv reads configuration from environment variables.
// Falls back to defaults if env vars are not set.
func ConfigFromEnv() Config {
	cfg := DefaultConfig()

	if v := os.Getenv("DSN_CHAIN_ID"); v != "" {
		if id, err := strconv.ParseUint(v, 10, 32); err == nil {
			cfg.ChainID = uint32(id)
		}
	}
	if v := os.Getenv("DSN_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("DSN_MEMPOOL_SIZE"); v != "" {
		if s, err := strconv.Atoi(v); err == nil && s > 0 {
			cfg.MempoolMaxSize = s
		}
	}
	if v := os.Getenv("DSN_RPC_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			cfg.RPCPort = p
		}
	}
	if v := os.Getenv("DSN_P2P_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			cfg.P2PPort = p
		}
	}
	if v := os.Getenv("DSN_INDEXER"); v != "" {
		cfg.IndexerEnabled = v == "true" || v == "1"
	}
	if v := os.Getenv("DSN_MAX_TX_PER_BLOCK"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxTxPerBlock = n
		}
	}
	if v := os.Getenv("DSN_PROPOSER_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.ProposerTimeout = d
		}
	}
	if v := os.Getenv("DSN_SNAPSHOT_INTERVAL"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			cfg.SnapshotInterval = n
		}
	}
	if v := os.Getenv("DSN_FAST_SYNC"); v != "" {
		cfg.FastSyncEnabled = v == "true" || v == "1"
	}
	if v := os.Getenv("DSN_TRUSTED_HEIGHT"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			cfg.TrustedCheckpointHeight = n
		}
	}
	if v := os.Getenv("DSN_TRUSTED_HASH"); v != "" {
		cfg.TrustedCheckpointHash = v
	}
	if v := os.Getenv("DSN_BOOTSTRAP_PEERS"); v != "" {
		// Comma-separated list of "ip:port" addresses
		cfg.BootstrapPeers = parseBootstrapPeers(v)
	}

	return cfg
}

// parseBootstrapPeers parses a comma-separated list of "ip:port" addresses.
func parseBootstrapPeers(s string) []string {
	if s == "" {
		return nil
	}
	var peers []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			peers = append(peers, p)
		}
	}
	return peers
}