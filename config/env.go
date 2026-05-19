package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// LoadConfigFromEnv loads configuration from environment variables with DSN_ prefix.
// Environment variables override file config values.
func LoadConfigFromEnv(cfg *Config) *Config {
	if cfg == nil {
		defaults := DefaultConfig()
		cfg = &defaults
	}

	// P2P config from environment
	if v := os.Getenv("DSN_P2P_LISTEN_ADDR"); v != "" {
		cfg.P2P.ListenAddr = v
	}
	if v := os.Getenv("DSN_P2P_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			cfg.P2P.Port = p
		}
	}
	if v := os.Getenv("DSN_P2P_MAX_PEERS"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			cfg.P2P.MaxPeers = p
		}
	}
	if v := os.Getenv("DSN_P2P_BOOTSTRAP_PEERS"); v != "" {
		cfg.P2P.BootstrapPeers = parseBootstrapPeers(v)
	}
	if v := os.Getenv("DSN_P2P_PING_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.P2P.PingInterval = d
		}
	}

	// RPC config from environment
	if v := os.Getenv("DSN_RPC_ENABLED"); v != "" {
		cfg.RPC.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("DSN_RPC_LISTEN_ADDR"); v != "" {
		cfg.RPC.ListenAddr = v
	}
	if v := os.Getenv("DSN_RPC_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			cfg.RPC.Port = p
		}
	}
	if v := os.Getenv("DSN_RPC_CORS_ORIGINS"); v != "" {
		cfg.RPC.CORSOrigins = strings.Split(v, ",")
	}

	// Metrics config from environment
	if v := os.Getenv("DSN_METRICS_ENABLED"); v != "" {
		cfg.Metrics.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("DSN_METRICS_LISTEN_ADDR"); v != "" {
		cfg.Metrics.ListenAddr = v
	}
	if v := os.Getenv("DSN_METRICS_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			cfg.Metrics.Port = p
		}
	}

	// Storage config from environment
	if v := os.Getenv("DSN_DATA_DIR"); v != "" {
		cfg.Storage.DataDir = v
	}
	if v := os.Getenv("DSN_MAX_DB_SIZE"); v != "" {
		if s, err := strconv.ParseInt(v, 10, 64); err == nil && s > 0 {
			cfg.Storage.MaxDBSize = s
		}
	}

	// Snapshot config from environment
	if v := os.Getenv("DSN_SNAPSHOT_ENABLED"); v != "" {
		cfg.Snapshot.Enable = v == "true" || v == "1"
	}
	if v := os.Getenv("DSN_SNAPSHOT_INTERVAL"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			cfg.Snapshot.Interval = n
		}
	}
	if v := os.Getenv("DSN_SNAPSHOT_MAX"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			cfg.Snapshot.MaxSnapshots = n
		}
	}
	if v := os.Getenv("DSN_SNAPSHOT_OUTPUT_DIR"); v != "" {
		cfg.Snapshot.OutputDir = v
	}

	// Validator config from environment
	if v := os.Getenv("DSN_VALIDATOR_KEY_FILE"); v != "" {
		cfg.Validator.KeyFile = v
	}
	if v := os.Getenv("DSN_VALIDATOR_STAKE"); v != "" {
		if s, err := strconv.ParseUint(v, 10, 64); err == nil {
			cfg.Validator.Stake = s
		}
	}
	if v := os.Getenv("DSN_VALIDATOR_COMMISSION_RATE"); v != "" {
		if r, err := strconv.ParseUint(v, 10, 64); err == nil {
			cfg.Validator.CommissionRate = r
		}
	}

	// Logging config from environment
	if v := os.Getenv("DSN_LOG_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
	if v := os.Getenv("DSN_LOG_FORMAT"); v != "" {
		cfg.Logging.Format = v
	}
	if v := os.Getenv("DSN_LOG_OUTPUT"); v != "" {
		cfg.Logging.Output = v
	}
	if v := os.Getenv("DSN_LOG_FILE_ENABLED"); v != "" {
		cfg.Logging.EnableFileLogging = v == "true" || v == "1"
	}

	// Chain config from environment
	if v := os.Getenv("DSN_CHAIN_ID"); v != "" {
		if id, err := strconv.ParseUint(v, 10, 32); err == nil {
			cfg.Chain.ChainID = uint32(id)
		}
	}
	if v := os.Getenv("DSN_MEMPOOL_SIZE"); v != "" {
		if s, err := strconv.Atoi(v); err == nil && s > 0 {
			cfg.Chain.MempoolMaxSize = s
		}
	}
	if v := os.Getenv("DSN_MEMPOOL_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.Chain.MempoolTTL = d
		}
	}
	if v := os.Getenv("DSN_MAX_TX_PER_BLOCK"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Chain.MaxTxPerBlock = n
		}
	}
	if v := os.Getenv("DSN_PROPOSER_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.Chain.ProposerTimeout = d
		}
	}
	if v := os.Getenv("DSN_INDEXER"); v != "" {
		cfg.Chain.IndexerEnabled = v == "true" || v == "1"
	}
	if v := os.Getenv("DSN_FAST_SYNC"); v != "" {
		cfg.Chain.FastSyncEnabled = v == "true" || v == "1"
	}
	if v := os.Getenv("DSN_TRUSTED_HEIGHT"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			cfg.Chain.TrustedCheckpointHeight = n
		}
	}
	if v := os.Getenv("DSN_TRUSTED_HASH"); v != "" {
		cfg.Chain.TrustedCheckpointHash = v
	}

	return cfg
}

// parseBootstrapPeers parses a comma-separated list of peer addresses.
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

// MergeConfig merges two configurations, with src overriding dst.
// Priority: CLI flags > env vars > config file > defaults
func MergeConfig(dst, src *Config) *Config {
	if dst == nil {
		defaults := DefaultConfig()
		dst = &defaults
	}
	if src == nil {
		return dst
	}

	// Only override with non-zero/non-empty values from src
	// This preserves the precedence: defaults < config file < env vars < CLI flags

	// P2P
	if src.P2P.ListenAddr != "" {
		dst.P2P.ListenAddr = src.P2P.ListenAddr
	}
	if src.P2P.Port != 0 {
		dst.P2P.Port = src.P2P.Port
	}
	if src.P2P.MaxPeers != 0 {
		dst.P2P.MaxPeers = src.P2P.MaxPeers
	}
	if len(src.P2P.BootstrapPeers) > 0 {
		dst.P2P.BootstrapPeers = src.P2P.BootstrapPeers
	}
	if src.P2P.PingInterval > 0 {
		dst.P2P.PingInterval = src.P2P.PingInterval
	}

	// RPC
	dst.RPC.Enabled = src.RPC.Enabled
	if src.RPC.ListenAddr != "" {
		dst.RPC.ListenAddr = src.RPC.ListenAddr
	}
	if src.RPC.Port != 0 {
		dst.RPC.Port = src.RPC.Port
	}
	if len(src.RPC.CORSOrigins) > 0 {
		dst.RPC.CORSOrigins = src.RPC.CORSOrigins
	}

	// Metrics
	dst.Metrics.Enabled = src.Metrics.Enabled
	if src.Metrics.ListenAddr != "" {
		dst.Metrics.ListenAddr = src.Metrics.ListenAddr
	}
	if src.Metrics.Port != 0 {
		dst.Metrics.Port = src.Metrics.Port
	}

	// Storage
	if src.Storage.DataDir != "" {
		dst.Storage.DataDir = src.Storage.DataDir
	}
	if src.Storage.MaxDBSize != 0 {
		dst.Storage.MaxDBSize = src.Storage.MaxDBSize
	}

	// Snapshot
	dst.Snapshot.Enable = src.Snapshot.Enable
	if src.Snapshot.Interval != 0 {
		dst.Snapshot.Interval = src.Snapshot.Interval
	}
	if src.Snapshot.MaxSnapshots != 0 {
		dst.Snapshot.MaxSnapshots = src.Snapshot.MaxSnapshots
	}
	if src.Snapshot.OutputDir != "" {
		dst.Snapshot.OutputDir = src.Snapshot.OutputDir
	}

	// Validator
	if src.Validator.KeyFile != "" {
		dst.Validator.KeyFile = src.Validator.KeyFile
	}
	if src.Validator.Stake != 0 {
		dst.Validator.Stake = src.Validator.Stake
	}
	if src.Validator.CommissionRate != 0 {
		dst.Validator.CommissionRate = src.Validator.CommissionRate
	}

	// Logging
	if src.Logging.Level != "" {
		dst.Logging.Level = src.Logging.Level
	}
	if src.Logging.Format != "" {
		dst.Logging.Format = src.Logging.Format
	}
	if src.Logging.Output != "" {
		dst.Logging.Output = src.Logging.Output
	}
	dst.Logging.EnableFileLogging = src.Logging.EnableFileLogging

	// Chain
	if src.Chain.ChainID != 0 {
		dst.Chain.ChainID = src.Chain.ChainID
	}
	if src.Chain.MempoolMaxSize != 0 {
		dst.Chain.MempoolMaxSize = src.Chain.MempoolMaxSize
	}
	if src.Chain.MempoolTTL > 0 {
		dst.Chain.MempoolTTL = src.Chain.MempoolTTL
	}
	if src.Chain.MaxTxPerBlock != 0 {
		dst.Chain.MaxTxPerBlock = src.Chain.MaxTxPerBlock
	}
	if src.Chain.ProposerTimeout > 0 {
		dst.Chain.ProposerTimeout = src.Chain.ProposerTimeout
	}
	dst.Chain.IndexerEnabled = src.Chain.IndexerEnabled
	dst.Chain.FastSyncEnabled = src.Chain.FastSyncEnabled
	if src.Chain.TrustedCheckpointHeight != 0 {
		dst.Chain.TrustedCheckpointHeight = src.Chain.TrustedCheckpointHeight
	}
	if src.Chain.TrustedCheckpointHash != "" {
		dst.Chain.TrustedCheckpointHash = src.Chain.TrustedCheckpointHash
	}

	return dst
}