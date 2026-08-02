package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLoadConfig_InitConfigToml verifies that the snake_case TOML produced by
// dev/localnet/init.sh decodes into the Config structs. Without explicit
// toml tags, BurntSushi/toml cannot map snake_case keys to Go field names,
// so every value would silently fall back to the default.
func TestLoadConfig_InitConfigToml(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[p2p]
listen_addr = "0.0.0.0"
port = 26656
max_peers = 50
bootstrap_peers = ["node0:26656"]
ping_interval = "30s"

[rpc]
enabled = true
listen_addr = "0.0.0.0"
port = 8545
cors_origins = []

[metrics]
enabled = true
listen_addr = "0.0.0.0"
port = 9464

[storage]
data_dir = "/root/.dsn/data"
max_db_size = 10737418240

[snapshot]
enable = true
interval = 10
max_snapshots = 5

[validator]
key_file = "/root/.dsn/validator.key"
stake = 1000000
commission_rate = 1000

[logging]
level = "info"
format = "text"
output = "stdout"

[chain]
chain_id = 0
mempool_max_size = 10000
mempool_ttl = "5m"
max_tx_per_block = 100
proposer_timeout = "5s"

[genesis]
file = "/root/genesis.json"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.P2P.ListenAddr != "0.0.0.0" {
		t.Errorf("P2P.ListenAddr = %q, want 0.0.0.0", cfg.P2P.ListenAddr)
	}
	if cfg.P2P.Port != 26656 {
		t.Errorf("P2P.Port = %d, want 26656", cfg.P2P.Port)
	}
	if cfg.P2P.MaxPeers != 50 {
		t.Errorf("P2P.MaxPeers = %d, want 50", cfg.P2P.MaxPeers)
	}
	if len(cfg.P2P.BootstrapPeers) != 1 || cfg.P2P.BootstrapPeers[0] != "node0:26656" {
		t.Errorf("P2P.BootstrapPeers = %v, want [node0:26656]", cfg.P2P.BootstrapPeers)
	}
	if cfg.P2P.PingInterval != 30*time.Second {
		t.Errorf("P2P.PingInterval = %s, want 30s", cfg.P2P.PingInterval)
	}

	if !cfg.RPC.Enabled {
		t.Error("RPC.Enabled = false, want true")
	}
	if cfg.RPC.ListenAddr != "0.0.0.0" {
		t.Errorf("RPC.ListenAddr = %q, want 0.0.0.0", cfg.RPC.ListenAddr)
	}
	if cfg.RPC.Port != 8545 {
		t.Errorf("RPC.Port = %d, want 8545", cfg.RPC.Port)
	}
	if len(cfg.RPC.CORSOrigins) != 0 {
		t.Errorf("RPC.CORSOrigins = %v, want []", cfg.RPC.CORSOrigins)
	}

	if !cfg.Metrics.Enabled {
		t.Error("Metrics.Enabled = false, want true")
	}
	if cfg.Metrics.ListenAddr != "0.0.0.0" {
		t.Errorf("Metrics.ListenAddr = %q, want 0.0.0.0", cfg.Metrics.ListenAddr)
	}
	if cfg.Metrics.Port != 9464 {
		t.Errorf("Metrics.Port = %d, want 9464", cfg.Metrics.Port)
	}

	if cfg.Storage.DataDir != "/root/.dsn/data" {
		t.Errorf("Storage.DataDir = %q, want /root/.dsn/data", cfg.Storage.DataDir)
	}
	if cfg.Storage.MaxDBSize != 10737418240 {
		t.Errorf("Storage.MaxDBSize = %d, want 10737418240", cfg.Storage.MaxDBSize)
	}

	if !cfg.Snapshot.Enable {
		t.Error("Snapshot.Enable = false, want true")
	}
	if cfg.Snapshot.Interval != 10 {
		t.Errorf("Snapshot.Interval = %d, want 10", cfg.Snapshot.Interval)
	}
	if cfg.Snapshot.MaxSnapshots != 5 {
		t.Errorf("Snapshot.MaxSnapshots = %d, want 5", cfg.Snapshot.MaxSnapshots)
	}

	if cfg.Validator.KeyFile != "/root/.dsn/validator.key" {
		t.Errorf("Validator.KeyFile = %q, want /root/.dsn/validator.key", cfg.Validator.KeyFile)
	}
	if cfg.Validator.Stake != 1000000 {
		t.Errorf("Validator.Stake = %d, want 1000000", cfg.Validator.Stake)
	}
	if cfg.Validator.CommissionRate != 1000 {
		t.Errorf("Validator.CommissionRate = %d, want 1000", cfg.Validator.CommissionRate)
	}

	if cfg.Logging.Level != "info" {
		t.Errorf("Logging.Level = %q, want info", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "text" {
		t.Errorf("Logging.Format = %q, want text", cfg.Logging.Format)
	}
	if cfg.Logging.Output != "stdout" {
		t.Errorf("Logging.Output = %q, want stdout", cfg.Logging.Output)
	}

	if cfg.Chain.ChainID != 0 {
		t.Errorf("Chain.ChainID = %d, want 0", cfg.Chain.ChainID)
	}
	if cfg.Chain.MempoolMaxSize != 10000 {
		t.Errorf("Chain.MempoolMaxSize = %d, want 10000", cfg.Chain.MempoolMaxSize)
	}
	if cfg.Chain.MempoolTTL != 5*time.Minute {
		t.Errorf("Chain.MempoolTTL = %s, want 5m", cfg.Chain.MempoolTTL)
	}
	if cfg.Chain.MaxTxPerBlock != 100 {
		t.Errorf("Chain.MaxTxPerBlock = %d, want 100", cfg.Chain.MaxTxPerBlock)
	}
	if cfg.Chain.ProposerTimeout != 5*time.Second {
		t.Errorf("Chain.ProposerTimeout = %s, want 5s", cfg.Chain.ProposerTimeout)
	}

	if cfg.Genesis.File != "/root/genesis.json" {
		t.Errorf("Genesis.File = %q, want /root/genesis.json", cfg.Genesis.File)
	}
}
