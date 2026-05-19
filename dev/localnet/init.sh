#!/usr/bin/env bash
set -euo pipefail

NODES=${1:-3}
BASE_DIR="$(cd "$(dirname "$0")" && pwd)"
TMP_DIR="$BASE_DIR/tmp"

echo "Initializing localnet with $NODES nodes..."

# Clean and create tmp directory
rm -rf "$TMP_DIR"
mkdir -p "$TMP_DIR"

# Build the dsn binary first
echo "Building dsn binary..."
cd "$BASE_DIR/../.."
if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed"
    exit 1
fi

# Build for linux (for docker)
GOOS=linux GOARCH=amd64 go build -o dsn ./cmd/dsn

# Move binary to tmp dir for local key generation
cp dsn "$TMP_DIR/"

# Go back to base dir
cd "$BASE_DIR"

# Generate validator keys and collect addresses
echo "Generating validator keys..."
VALIDATORS=""
for i in $(seq 0 $((NODES - 1))); do
    mkdir -p "$TMP_DIR/node$i"

    # Generate validator key using dsn binary
    "$TMP_DIR/dsn" validator init --output "$TMP_DIR/node$i/validator.key" --force

    # Extract address and pubkey from the generated key file
    KEY_JSON=$(cat "$TMP_DIR/node$i/validator.key")
    ADDR=$(echo "$KEY_JSON" | grep -o '"address":"[^"]*"' | cut -d'"' -f4)
    PUBKEY=$(echo "$KEY_JSON" | grep -o '"public_key":"[^"]*"' | cut -d'"' -f4)

    # Calculate P2P and RPC ports
    P2P_PORT=$((26656 + i))
    RPC_PORT=$((8545 + i))
    METRICS_PORT=$((9464 + i))

    # Node-specific IP (for docker network)
    NODE_IP="10.0.1.$((10 + i))"

    # Bootstrap peers: node0 is the seed, others connect to it
    if [ $i -eq 0 ]; then
        BOOTSTRAP_PEERS="[]"
    else
        BOOTSTRAP_PEERS='["node0:26656"]'
    fi

    # Create node config
    cat > "$TMP_DIR/node$i/config.toml" <<CONF
[p2p]
listen_addr = "0.0.0.0"
port = $P2P_PORT
max_peers = 50
bootstrap_peers = $BOOTSTRAP_PEERS
ping_interval = "30s"

[rpc]
enabled = true
listen_addr = "0.0.0.0"
port = $RPC_PORT
cors_origins = []

[metrics]
enabled = true
listen_addr = "0.0.0.0"
port = $METRICS_PORT

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
CONF

    # Add validator to genesis list
    if [ $i -gt 0 ]; then
        VALIDATORS="$VALIDATORS,"
    fi
    # Commission as string (JSON string, not number)
    VALIDATORS="$VALIDATORS{\"address\":\"$ADDR\",\"pub_key\":\"$PUBKEY\",\"consensus_key\":\"$PUBKEY\",\"stake\":1000000,\"commission\":\"1000\"}"

    echo "  Node $i: $ADDR (P2P: $P2P_PORT, RPC: $RPC_PORT)"
done

# Generate genesis.json with all validators
GENESIS_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)

cat > "$TMP_DIR/genesis.json" <<GENESIS
{
    "genesis_time": "$GENESIS_TIME",
    "chain_id": "dsn-localnet-1",
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
    "initial_validators": [$VALIDATORS],
    "initial_balances": [],
    "treasury": {
        "address": "0000000000000000000000000000000000000000",
        "initial_balance": 0
    }
}
GENESIS

# Clean up the temporary dsn binary
rm -f "$TMP_DIR/dsn"

echo ""
echo "Localnet initialized successfully!"
echo "  Nodes: $NODES"
echo "  Genesis: $TMP_DIR/genesis.json"
echo "  Node data: $TMP_DIR/node{0,1,2}/"
echo ""
echo "To start the localnet:"
echo "  cd $TMP_DIR"
echo "  docker-compose -f $BASE_DIR/docker-compose.yml up -d"
echo ""
echo "To stop:"
echo "  docker-compose -f $BASE_DIR/docker-compose.yml down"