package config

import (
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
