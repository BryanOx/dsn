# DSN Node — Setup Guide

## Prerequisites

- **Go 1.21+** — check `go.mod` (script uses 1.25, but Dockerfile builds with `1.21-alpine`)
- **Git**
- **No C dependencies** — BoltDB and WASM (via wazero) are pure Go

## Build

```bash
# Build both binaries (node/CLI + TUI)
make build

# Or manually:
go build -ldflags="-X main.Version=$(cat VERSION)" -o bin/dsn.exe ./cmd/dsn
go build -ldflags="-X main.Version=$(cat VERSION)" -o bin/dsn-tui.exe ./cmd/dsn-tui
```

| Binary | Description |
|--------|-------------|
| `bin/dsn.exe` | Node + CLI |
| `bin/dsn-tui.exe` | Terminal UI (client) |

## Devnet (single node, in-memory)

The fastest way to test:

```bash
dsn devnet
```

This:
- Creates in-memory state with a random validator key
- Pre-funds the validator with 1,000,000 DSN
- Produces blocks every **3 seconds** (hardcoded, ignores `DSN_BLOCK_TIME_SEC`)
- Starts JSON-RPC on port **8545**
- Starts Explorer API on port **8080**
- Starts Prometheus metrics on port **9464**
- Disables P2P (single node)

### Optional flags

| Flag | Default | Description |
|------|---------|-------------|
| `--rpc-port` | `8545` | RPC server port |
| `--explorer-port` | `8080` | Explorer API port |

## Mainnet / Production

Requires a genesis file and a validator key.

### Generate genesis file

```bash
# Devnet-style genesis (auto-generates N validator keys)
dsn genesis init --devnet --validators 3 --chain-id mynet-1 --output genesis.json

# With specific validator addresses
dsn genesis init --validator-addresses <addr1>,<addr2> --chain-id mainnet-1

# Or use prepared templates:
#   deploy/mainnet/genesis-template.json
#   deploy/sandbox/genesis.json
```

### Generate validator key

```bash
dsn validator init --output ~/.dsn/validator.key
```

### Start the node

```bash
dsn node start \
  --genesis genesis.json \
  --validator-key ~/.dsn/validator.key \
  --data-dir ./data
```

Override the default RPC port if needed:

```bash
dsn node start \
  --genesis genesis.json \
  --validator-key ~/.dsn/validator.key \
  --data-dir ./data \
  --rpc-port 8545
```

## Default Ports Summary

| Service | Port | Notes |
|---------|------|-------|
| JSON-RPC (HTTP) | 8545 | Main query / submit endpoint |
| P2P | 0 (disabled) | Enable for multi-node |
| Metrics (Prometheus) | 9464 | Health / performance metrics |
| Devnet Explorer API | 8080 | Devnet-only faucet / explorer |

## Environment Variables

All configuration can be set via `DSN_*` environment variables. Precedence (high to low):

> CLI flags > Environment variables > TOML config > Built-in defaults

| Variable | Default | Description |
|----------|---------|-------------|
| `DSN_RPC_PORT` | `8545` | RPC server port |
| `DSN_P2P_PORT` | `0` | P2P port (`0` = disabled) |
| `DSN_METRICS_PORT` | `9464` | Metrics port |
| `DSN_DATA_DIR` | `""` | Data directory (empty = in-memory) |
| `DSN_VALIDATOR_KEY_FILE` | `validator_key.json` | Validator key path |
| `DSN_CHAIN_ID` | `0` | Network chain ID |
| `DSN_LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `DSN_BLOCK_TIME_SEC` | `1` | Block time in seconds (production; devnet uses hardcoded 3s) |
| `DSN_MEMPOOL_SIZE` | `10000` | Mempool max capacity |
| `DSN_MAX_TX_PER_BLOCK` | `100` | Max transactions per block |
| `DSN_INDEXER` | `false` | Enable block indexer |
| `DSN_FAST_SYNC` | `false` | Enable fast sync |

> **Full list:** See `config/env.go` and `node/config.go` for 30+ available variables including P2P networking, snapshots, TLS, and advanced tuning.

## Docker Deployment

### Localnet (3 validators)

```bash
cd dev/localnet
docker-compose up -d
```

| Resource | Details |
|----------|---------|
| P2P ports | 26656, 26657, 26658 |
| RPC ports | 8545, 8546, 8547 |
| Network | 10.0.1.0/24 |

### Sandbox (3 validators + monitoring)

```bash
cd deploy/sandbox
docker-compose up -d
```

| Resource | Details |
|----------|---------|
| P2P ports | 30333, 30334, 30335 |
| RPC ports | 8545, 8546, 8547 |
| Additional services | Prometheus (`:9090`), Grafana (`:3000`) |

## Systemd Service (Linux)

```bash
sudo cp deploy/systemd/dsn.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable dsn
sudo systemctl start dsn
```

Configuration goes in `/etc/dsn/config.env`.
