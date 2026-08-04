// Package localnet provides tools for setting up local development networks.
package localnet

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	wallet2 "github.com/BryanOx/dsn/wallet"
)

// Config represents localnet configuration options.
type Config struct {
	NumValidators int
	OutputDir     string
	ChainID       string
}

// DefaultConfig returns sensible defaults for localnet.
func DefaultConfig() Config {
	return Config{
		NumValidators: 3,
		OutputDir:     "dev/localnet/tmp",
		ChainID:       "dsn-localnet-1",
	}
}

// NodeConfig represents per-node configuration.
type NodeConfig struct {
	Index       int
	Address     string
	PublicKey   string
	P2PPort     int
	RPCPort     int
	MetricsPort int
	IP          string
}

// GenerateLocalnet creates a localnet with the specified configuration.
func GenerateLocalnet(cfg Config) ([]NodeConfig, error) {
	if cfg.NumValidators < 1 {
		return nil, fmt.Errorf("at least 1 validator required, got %d", cfg.NumValidators)
	}

	// Create output directory
	if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	nodes := make([]NodeConfig, cfg.NumValidators)
	validators := make([]map[string]interface{}, cfg.NumValidators)

	// Generate validator keys
	for i := 0; i < cfg.NumValidators; i++ {
		nodeDir := filepath.Join(cfg.OutputDir, fmt.Sprintf("node%d", i))
		if err := os.MkdirAll(nodeDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create node directory: %w", err)
		}

		// Generate validator key
		kp, err := wallet2.GenerateValidatorKey()
		if err != nil {
			return nil, fmt.Errorf("failed to generate validator key: %w", err)
		}

		// Save validator key
		keyPath := filepath.Join(nodeDir, "validator.key")
		if err := wallet2.SaveValidatorKey(keyPath, kp.PrivateKey, kp.PublicKey); err != nil {
			return nil, fmt.Errorf("failed to save validator key: %w", err)
		}

		// Create node config
		nodes[i] = NodeConfig{
			Index:       i,
			Address:     hex.EncodeToString(kp.Address[:]),
			PublicKey:   hex.EncodeToString(kp.PublicKey),
			P2PPort:     26656,
			RPCPort:     8545,
			MetricsPort: 9464,
			IP:          fmt.Sprintf("10.0.1.%d", 10+i),
		}

		// Add to validators for genesis
		validators[i] = map[string]interface{}{
			"address":       hex.EncodeToString(kp.Address[:]),
			"pub_key":       hex.EncodeToString(kp.PublicKey),
			"consensus_key": hex.EncodeToString(kp.PublicKey),
			"stake":         1000000,
			"commission":    "1000",
		}

		// Generate node config file
		if err := generateNodeConfig(nodeDir, nodes[i], cfg.ChainID); err != nil {
			return nil, fmt.Errorf("failed to generate node config: %w", err)
		}

		fmt.Printf("Generated node %d: %s\n", i, kp.Address.String())
	}

	// Generate genesis.json
	if err := generateGenesis(cfg.OutputDir, cfg.ChainID, validators); err != nil {
		return nil, fmt.Errorf("failed to generate genesis: %w", err)
	}

	fmt.Printf("\nLocalnet generated: %d nodes\n", cfg.NumValidators)
	fmt.Printf("Output directory: %s\n", cfg.OutputDir)
	fmt.Println("\nTo start the localnet:")
	fmt.Printf("  cd %s\n", cfg.OutputDir)
	fmt.Printf("  docker-compose -f %s/../docker-compose.yml up -d\n", filepath.Join(cfg.OutputDir, ".."))

	return nodes, nil
}

// generateNodeConfig creates a TOML config file for a node.
func generateNodeConfig(nodeDir string, node NodeConfig, chainID string) error {
	p2pBootstrapPeers := "[]"
	if node.Index > 0 {
		p2pBootstrapPeers = `["node0:26656"]`
	}

	config := fmt.Sprintf(`[p2p]
listen_addr = "0.0.0.0"
port = %d
max_peers = 50
bootstrap_peers = %s
ping_interval = "30s"

[rpc]
enabled = true
listen_addr = "0.0.0.0"
port = %d
cors_origins = []

[metrics]
enabled = true
listen_addr = "0.0.0.0"
port = %d

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

[genesis]
file = "/root/genesis.json"
`, node.P2PPort, p2pBootstrapPeers, node.RPCPort, node.MetricsPort)

	configPath := filepath.Join(nodeDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// generateGenesis creates the genesis.json file.
func generateGenesis(outputDir, chainID string, validators []map[string]interface{}) error {
	validatorsJSON, err := json.Marshal(validators)
	if err != nil {
		return fmt.Errorf("failed to marshal validators: %w", err)
	}

	genesis := fmt.Sprintf(`{
    "genesis_version": 1,
    "genesis_time": "%s",
    "chain_id": "%s",
    "initial_height": 1,
    "consensus_params": {
        "max_tx_per_block": 100,
        "max_bytes_per_block": 1048576,
        "max_gas_per_block": 10000000
    },
    "epoch_params": {
        "blocks_per_epoch": 10,
        "unstake_cooldown_epochs": 1,
        "max_validators": 50,
        "minimum_stake": 100000
    },
    "inflation_params": {
        "enabled": false
    },
    "initial_validators": %s,
    "initial_balances": [],
    "treasury": {
        "address": "0000000000000000000000000000000000000000",
        "initial_balance": 0
    }
}`, time.Now().UTC().Format(time.RFC3339), chainID, string(validatorsJSON))

	genesisPath := filepath.Join(outputDir, "genesis.json")
	if err := os.WriteFile(genesisPath, []byte(genesis), 0644); err != nil {
		return fmt.Errorf("failed to write genesis: %w", err)
	}

	return nil
}
