package node

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/BryanOx/dsn/types"
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

	// MaxRound caps the consensus view-change round index. On the round
	// deadline the node advances its pending round, but never past MaxRound —
	// at the cap it keeps waiting and re-broadcasting instead of advancing.
	// Defaults to 8.
	MaxRound uint32

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

	// TLSCertFile is the path to the TLS certificate file.
	// When set, the RPC server will serve HTTPS instead of HTTP.
	TLSCertFile string

	// TLSKeyFile is the path to the TLS private key file.
	TLSKeyFile string

	// RPCApiKey is an optional API key required for all RPC requests.
	// When set, requests without a matching X-API-Key header are rejected.
	RPCApiKey string

	// RPCApiKeyHeader is the header name for API key auth (default: X-API-Key).
	RPCApiKeyHeader string

	// BlockTimeSec is the block time in seconds for economic calculations (default: 1).
	BlockTimeSec uint64
}

func DefaultConfig() Config {
	return Config{
		ChainID:                 0,
		DataDir:                 "", // empty = in-memory only. Set to a path for persistent storage.
		MempoolMaxSize:          10000,
		MempoolTTL:              300 * time.Second,
		RPCPort:                 8545,
		P2PPort:                 0, // 0 = P2P disabled (use env var DSN_P2P_PORT to enable)
		Validators:              []types.Address{},
		MaxTxPerBlock:           100,
		ProposerTimeout:         6 * time.Second,
		MaxRound:                8,
		GenesisFile:             "",    // genesis file path
		ValidatorKeyFile:        "",    // validator key file path
		MaxPeers:                50,    // max P2P peers
		MetricsPort:             9464,  // Prometheus metrics port
		IndexerEnabled:          false, // disabled by default
		SnapshotInterval:        10,    // snapshot every 10 epochs
		FastSyncEnabled:         true,  // snapshot fast-sync for fresh nodes
		TrustedCheckpointHeight: 0,
		TrustedCheckpointHash:   "",
		VMTimeoutSeconds:        30,          // 30 second VM execution timeout
		FSync:                   false,       // async writes by default
		TLSCertFile:             "",          // no TLS by default
		TLSKeyFile:              "",          // no TLS by default
		RPCApiKey:               "",          // no API key by default
		RPCApiKeyHeader:         "X-API-Key", // default header name
		BlockTimeSec:            1,           // 1 second block time by default
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
		// S9: a timeout base at or below the ±5s clock-skew window
		// (types.MaxTimestampDrift) cannot distinguish a stalled proposer from
		// a stale block, so it is rejected by keeping the default.
		if d, err := time.ParseDuration(v); err == nil && d > types.MaxTimestampDrift {
			cfg.ProposerTimeout = d
		}
	}
	if v := os.Getenv("DSN_MAX_ROUND"); v != "" {
		// A MaxRound of 0 would pin the pending round at 0 forever (no view
		// change possible), so it is rejected by keeping the default — the
		// same fallback used for the ≤5s timeout base above.
		if r, err := strconv.ParseUint(v, 10, 32); err == nil && r >= 1 {
			cfg.MaxRound = uint32(r)
		} else if err == nil {
			log.Printf("[config] DSN_MAX_ROUND=%q rejected: MaxRound must be >= 1; keeping default %d", v, cfg.MaxRound)
		}
	}
	if v := os.Getenv("DSN_SNAPSHOT_INTERVAL"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			cfg.SnapshotInterval = n
		}
	}
	if v := os.Getenv("DSN_BLOCK_TIME_SEC"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil && n > 0 {
			cfg.BlockTimeSec = n
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
	if v := os.Getenv("DSN_TLS_CERT_FILE"); v != "" {
		cfg.TLSCertFile = v
	}
	if v := os.Getenv("DSN_TLS_KEY_FILE"); v != "" {
		cfg.TLSKeyFile = v
	}
	if v := os.Getenv("DSN_RPC_API_KEY"); v != "" {
		cfg.RPCApiKey = v
	}
	if v := os.Getenv("DSN_RPC_API_KEY_HEADER"); v != "" {
		cfg.RPCApiKeyHeader = v
	}

	return cfg
}

// parseBootstrapPeers parses a comma-separated list of "ip:port" addresses.
// Duplicate entries are collapsed, preserving first-seen order.
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
	return dedupeBootstrapPeers(peers)
}

// dedupeBootstrapPeers collapses duplicate addresses, preserving the order of
// first occurrence. Duplicate bootstrap peers would make peer discovery dial
// (and re-dial) the same address twice every backoff interval.
func dedupeBootstrapPeers(peers []string) []string {
	seen := make(map[string]struct{}, len(peers))
	result := make([]string, 0, len(peers))
	for _, p := range peers {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		result = append(result, p)
	}
	return result
}
