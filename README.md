# DSN — Deterministic Settlement Network v0.1.0-sandbox

[![Go Report Card](https://goreportcard.com/badge/github.com/BryanOx/dsn)](https://goreportcard.com/report/github.com/BryanOx/dsn)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

DSN is a **BFT blockchain for settlement finality** with deterministic WASM smart contracts, Ed25519 signing, and Sparse Merkle Tree state commitment. Built in Go 1.25 with BoltDB storage, custom TCP networking, and a pipelined voting consensus model.

**v0.1.0-sandbox** — functional but pre-production. Core features work for local devnets and single-node operation. Several areas remain stubbed or incomplete (see [Known Limitations](docs/OPERATIONS.md#known-limitations)).

## Quick Start

```bash
go build -o dsn.exe ./cmd/dsn
dsn genesis init --devnet --validators 3 --chain-id sandbox-1
dsn node start --config config.toml --genesis genesis.json --validator-key validator-0.json --data-dir data/
```

See [docs/GUIDES.md#quickstart](docs/GUIDES.md#quickstart) for the full walkthrough.

## Features

- **BFT Consensus** — Pipelined three-phase voting with round-robin proposer rotation and evidence-based slashing
- **WASM Smart Contracts** — Deterministic WebAssembly 1.0 runtime via [wazero](https://github.com/tetratelabs/wazero) (7 host functions: storage read/write, events, caller/block info, token transfers)
- **Ed25519 Cryptography** — Transaction signing and validator identity
- **Sparse Merkle Tree** — Cryptographic state commitment with 32-byte roots, InMemory + BoltDB persistence
- **BoltDB Storage** — ACID-compliant, pure Go, no CGO — with WAL and snapshot-based recovery
- **Gossip Networking** — Custom TCP protocol with bootstrap peer discovery (no libp2p)
- **Validator Lifecycle** — Registration, staking, epoch-based activation, rewards, jailing, and slashing
- **JSON-RPC 2.0 API** — 17 methods for transaction submission, state queries, and contract interaction
- **Devnet Mode** — Built-in multi-node environment with faucet, block explorer, and indexer
- **TUI Client** — Terminal dashboard with 5 tabs (Dashboard, Balance, Send, Pending, Wallet)
- **Multi-Platform** — Linux, macOS, Windows

## Documentation

| Guide | Description |
|-------|-------------|
| [Quick Start](docs/GUIDES.md#quickstart) | From clone to running a 3-validator devnet |
| [Node Operation](docs/OPERATIONS.md#node-operation) | Starting, configuring, and managing nodes |
| [Validator Guide](docs/GUIDES.md#validator-guide) | Registration, staking, rewards, and slashing |
| [TUI Guide](docs/GUIDES.md#tui-guide) | Terminal UI dashboard and wallet interactions |
| [Wallet Guide](docs/GUIDES.md#wallet-guide) | Key generation, signing, and transaction management |
| [Contracts](docs/GUIDES.md#smart-contracts) | Deploying and calling WASM smart contracts |
| [CLI Reference](docs/GUIDES.md#cli-reference) | Complete command reference with examples |
| [Architecture](docs/ARCHITECTURE.md) | System design, components, and data flow |
| [Networking](docs/ARCHITECTURE.md#networking--p2p) | P2P gossip protocol and peer discovery |
| [Security Model](docs/ARCHITECTURE.md#security-model) | Threat model, trust assumptions, and crypto |
| [Operations](docs/OPERATIONS.md) | Deployment, monitoring, and recovery |
| [Sandbox Deployment](docs/OPERATIONS.md#sandbox-deployment) | Docker Compose multi-node setup |
| [Troubleshooting](docs/OPERATIONS.md#troubleshooting) | Common issues and solutions |
| [Known Limitations](docs/OPERATIONS.md#known-limitations) | What is and isn't working in v0.1.0-sandbox |

## Package Structure

```
cmd/dsn/           CLI entry point (node, wallet, validator, contract commands)
cmd/dsn-tui/       Terminal UI binary (5-tab dashboard)
types/             Core types: Address, Hash, Amount, Transaction, Block
state/             Sparse Merkle Tree, account state, BoltDB persistence
mempool/           Transaction memory pool
wallet/            Ed25519 key management and signing
network/           P2P networking (TCP, custom protocol, gossip, discovery)
consensus/         BFT consensus, block production, finality
staking/           Validator staking, rewards, slashing, epochs
node/              Node orchestrator (startup, shutdown, recovery)
rpc/               JSON-RPC 2.0 server (17 methods)
vm/                WASM smart contract runtime (wazero, deterministic)
genesis/           Genesis loading and validation (8 validation rules)
config/            Configuration (TOML, env vars, CLI flags)
telemetry/         Logging and metrics
indexer/           Block and transaction indexing
explorer/          Block explorer HTTP server
sdk/               Go SDK for client interaction
docs/              Documentation
benchmarks/        Performance benchmarks
integration/       Integration tests
dev/localnet/      Docker Compose multi-node devnet
```

## CLI Commands

```
dsn node start --config <toml> --genesis <path> [--validator-key <path>] [--data-dir <path>]
dsn genesis init [--devnet] [--validators N] [--chain-id <id>] [--output <path>]
dsn genesis validate [--file <path>]
dsn genesis devnet [--validators N] [--output <dir>]
dsn validator init [--output <path>]
dsn validator register --key <path> --stake <amount> --commission <rate> [--node <url>]
dsn validator status [--key <path>] [--node <url>]
dsn wallet generate [--output <path>]
dsn wallet sign <tx-file> [--key <path>]
dsn wallet nonce <address> [--rpc <url>]
dsn contract deploy <wasm-path> [--key <path>] [--gas-limit <n>] [--rpc <url>]
dsn contract call <addr> <entrypoint> [--data <hex>] [--rpc <url>]
dsn contract estimate <addr> <entrypoint> [--data <hex>] [--rpc <url>]
dsn contract query <addr> <key> [--rpc <url>]
dsn contract receipt <txid> [--rpc <url>]
dsn devnet [--rpc-port <n>] [--explorer-port <n>] [--indexer] [--reset]
```

## Building

```bash
# Binary
go build -o dsn.exe ./cmd/dsn
go build -o dsn-tui.exe ./cmd/dsn-tui

# Both to bin/ via Make
make build

# Tests
go test ./... -cover -timeout 120s
```

## Configuration

Hierarchical (lowest to highest precedence):

```
defaults < TOML file < environment variables (DSN_*) < CLI flags
```

```bash
# TOML
rpc_port = 8545

# Env override
$env:DSN_RPC_PORT = "8546"

# CLI override (highest)
dsn node start --config config.toml --rpc-port 8547
```

## License

MIT — see [LICENSE](LICENSE) for details.
