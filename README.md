# DSN — Deterministic Settlement Network

[![Build Status](https://github.com/dsn/dsn/actions/workflows/ci.yml/badge.svg)](https://github.com/dsn/dsn/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/dsn/dsn)](https://goreportcard.com/report/github.com/dsn/dsn)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

DSN is a **financial-grade deterministic blockchain** purpose-built for settlement finality. It guarantees byte-identical execution across all nodes through BFT consensus, deterministic WASM runtime, and provable state transitions.

## Quick Start

[See docs/QUICKSTART.md](docs/QUICKSTART.md) for a step-by-step guide to running a local 3-validator network.

## Architecture Overview

[See docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for system architecture, component design, and C4-style diagrams.

## Documentation

| Document | Description |
|----------|-------------|
| [Protocol Spec](docs/dsn_protocol_spec_v_1.md) | Full protocol specification (881 lines) |
| [Whitepaper](docs/WHITEPAPER.md) | Design goals and rationale |
| [Architecture](docs/ARCHITECTURE.md) | System architecture and component design |
| [Quick Start](docs/QUICKSTART.md) | From clone to running network |
| [Contract Guide](docs/CONTRACTS.md) | Deploying WASM smart contracts |
| [Operations](docs/OPERATIONS.md) | Deployment and monitoring |
| [Recovery Runbook](docs/RECOVERY_RUNBOOK.md) | Incident recovery procedures |
| [SDK Guide](docs/SDK_GUIDE.md) | Go SDK for client interaction |
| [Examples](docs/EXAMPLES.md) | Example contracts (Token, Escrow, Settlement) |
| [Release Notes](docs/RELEASE_NOTES.md) | Release details and features |
| [Known Limitations](docs/SANDBOX_LIMITATIONS.md) | Sandbox limitations and disclaimers |
| [Final Verification](docs/FINAL_VERIFICATION.md) | Verification checklist |

## Features

- **Deterministic Execution** — Byte-identical state transitions across all nodes; every validator produces the same result
- **BFT Consensus** — Byzantine fault tolerance with evidence-based slashing for validator accountability
- **WASM Runtime** — Deterministic smart contracts via [wazero](https://github.com/tetratelabs/wazero)
- **Staking Economics** — Inflation-based rewards (10% treasury, 90% validator pool) with epoch-based distribution
- **Sparse Merkle Trees** — Cryptographic state commitment for every state transition
- **Persistence & Recovery** — BoltDB-backed storage with WAL and snapshots for crash recovery
- **Operational Tooling** — Helm charts, systemd units, Prometheus/Grafana monitoring

## Project Structure

```
dsn/
├── cmd/dsn/              # CLI entry point (node, wallet, validator commands)
├── cmd/dsn-tui/          # Terminal UI for node interaction
├── types/                # Core types: Address, Hash, Amount, Transaction, Block
├── state/                # SMT storage, account state, persistence (BoltDB)
├── mempool/              # Transaction memory pool with fee-based ordering
├── wallet/               # Ed25519 key management and transaction signing
├── network/              # P2P networking (TCP, custom protocol, gossip)
├── consensus/            # BFT consensus, block production, finality
├── staking/              # Validator staking, rewards, slashing, epochs
├── node/                 # Node orchestrator (startup, shutdown, recovery)
├── rpc/                  # JSON-RPC 2.0 server for external interaction
├── vm/                   # WASM smart contract runtime (wazero)
├── genesis/              # Genesis block creation and validation
├── config/               # Configuration management (TOML, env vars, CLI)
├── telemetry/            # Logging, metrics, middleware
├── indexer/              # Block and transaction indexing
├── explorer/             # Block explorer HTTP server
├── sdk/                  # Go SDK for client interaction
├── docs/                 # Protocol specification and whitepaper
├── benchmarks/          # Performance benchmarks
└── integration/         # Integration tests
```

## Key Technologies

| Component | Technology | Rationale |
|-----------|------------|-----------|
| Language | Go 1.25 | Concurrent, simple, battle-tested |
| State | Sparse Merkle Tree | Cryptographic commitment, 32-byte roots |
| Storage | BoltDB | Pure Go, no CGO, ACID-compliant |
| WASM | wazero | Deterministic, sandboxed execution |
| Crypto | Ed25519 | Modern, fast, well-audited |
| Networking | Custom TCP | Avoid libp2p Windows compatibility issues |

## Building

```bash
# Clone and build
git clone https://github.com/dsn/dsn.git
cd dsn
go build -o dsn ./cmd/dsn

# Run tests
go test ./... -cover -timeout 120s

# Run linter
go vet ./...
```

## Configuration

Configuration precedence (lowest to highest):
```
defaults < TOML file < environment variables (DSN_*) < CLI flags
```

Example:
```bash
# TOML (config.toml)
rpc_port = 8545

# Override via env
export DSN_RPC_PORT=8546

# Override via CLI
./dsn node start --rpc-port 8547
```

## License

MIT License — see [LICENSE](LICENSE) for details.