package config

import (
	"reflect"
	"testing"
)

// TestLoadConfigFromEnv_GenesisEnvVar tests that DSN_GENESIS sets
// cfg.Genesis.File.
func TestLoadConfigFromEnv_GenesisEnvVar(t *testing.T) {
	t.Setenv("DSN_GENESIS", "/tmp/dsn-test/genesis.json")

	cfg := LoadConfigFromEnv(nil)

	if cfg.Genesis.File != "/tmp/dsn-test/genesis.json" {
		t.Errorf("Genesis.File = %q, want %q", cfg.Genesis.File, "/tmp/dsn-test/genesis.json")
	}
}

// TestLoadConfigFromEnv_ValidatorKeyFileEnvVar tests that
// DSN_VALIDATOR_KEY_FILE sets cfg.Validator.KeyFile.
func TestLoadConfigFromEnv_ValidatorKeyFileEnvVar(t *testing.T) {
	t.Setenv("DSN_VALIDATOR_KEY_FILE", "/tmp/dsn-test/validator.key")

	cfg := LoadConfigFromEnv(nil)

	if cfg.Validator.KeyFile != "/tmp/dsn-test/validator.key" {
		t.Errorf("Validator.KeyFile = %q, want %q", cfg.Validator.KeyFile, "/tmp/dsn-test/validator.key")
	}
}

// TestLoadConfigFromEnv_BootstrapPeersReplacesToml verifies the precedence
// contract: when the same bootstrap peer is configured in both the config file
// and DSN_P2P_BOOTSTRAP_PEERS, the env var wins and the final list has exactly
// one entry. A duplicate here would make peer discovery dial (and re-dial) the
// same address twice every backoff interval.
func TestLoadConfigFromEnv_BootstrapPeersReplacesToml(t *testing.T) {
	t.Setenv("DSN_P2P_BOOTSTRAP_PEERS", "node0:26656")

	// Simulate a config file that already lists the same bootstrap peer.
	fileCfg := &Config{
		P2P: P2PConfig{BootstrapPeers: []string{"node0:26656"}},
	}

	cfg := LoadConfigFromEnv(fileCfg)

	if len(cfg.P2P.BootstrapPeers) != 1 || cfg.P2P.BootstrapPeers[0] != "node0:26656" {
		t.Errorf("P2P.BootstrapPeers = %v, want [node0:26656]", cfg.P2P.BootstrapPeers)
	}
}

// TestLoadConfigFromEnv_BootstrapPeersDedupes verifies that duplicates from
// either source (config file or env var) are collapsed, preserving order.
func TestLoadConfigFromEnv_BootstrapPeersDedupes(t *testing.T) {
	t.Setenv("DSN_P2P_BOOTSTRAP_PEERS", "node0:26656,node1:26656,node0:26656")

	cfg := LoadConfigFromEnv(nil)

	want := []string{"node0:26656", "node1:26656"}
	if !reflect.DeepEqual(cfg.P2P.BootstrapPeers, want) {
		t.Errorf("P2P.BootstrapPeers = %v, want %v", cfg.P2P.BootstrapPeers, want)
	}
}
