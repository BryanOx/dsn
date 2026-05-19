# DSN Sandbox Devnet

## Overview

The DSN Sandbox is a public test network for developers to experiment with DSN's deterministic settlement capabilities.

## Network Info

| Parameter | Value |
|-----------|-------|
| Chain ID | dsn-sandbox-1 |
| RPC URL | https://rpc.sandbox.dsn.io |
| Explorer | https://explorer.sandbox.dsn.io |
| Faucet | https://faucet.sandbox.dsn.io |
| Chain Type | Testnet (sandbox) |

## Infrastructure

### Bootstrap Nodes

- `/dns/bootstrap-0.sandbox.dsn.io/tcp/30333/p2p/12D3KooWH9VtkxRMq7uGZKkEGN2vWz9vKf7NzN9L4vX5yZqPu6T`
- `/dns/bootstrap-1.sandbox.dsn.io/tcp/30333/p2p/12D3KooWJ8YJG3qN6s6vY4tK9mZ7L5pG8kP2fB3mR4vX6yZqPe9U`

### Validator Nodes

4 validators operated by the DSN team. Community validators welcome to join.

## Quick Start

### 1. Install DSN

```bash
git clone https://github.com/dsn/dsn.git
cd dsn
make install
```

### 2. Initialize Node

```bash
dsn init --network sandbox --data-dir ~/.dsn-sandbox
```

### 3. Start Node

```bash
dsn start --data-dir ~/.dsn-sandbox
```

### 4. Get Test Tokens

Visit https://faucet.sandbox.dsn.io and enter your address.

### 5. Deploy a Contract

```bash
dsn tx wasm deploy ./counter.wasm --from my-wallet
```

## Local Development with Docker

Run a local 3-validator sandbox cluster:

```bash
cd deploy/sandbox
docker-compose up -d
```

Available services:

| Service | Port | Description |
|---------|------|-------------|
| validator-0 RPC | 8545 | Primary validator JSON-RPC |
| validator-1 RPC | 8546 | Secondary validator |
| validator-2 RPC | 8547 | Tertiary validator |
| P2P (validator-0) | 30333 | P2P networking |
| P2P (validator-1) | 30334 | P2P networking |
| P2P (validator-2) | 30335 | P2P networking |

### Access Local Sandbox

```bash
# Check validator status
curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"dsn_getStatus","params":[],"id":1}'

# Check balance
curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"dsn_getBalance","params":["faucet0000000000000000000000000000000001"],"id":2}'

# Get state root
curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"dsn_getStateRoot","params":[],"id":3}'
```

Stop the local cluster:

```bash
docker-compose down -v  # -v removes volumes
```

## Faucet

The sandbox faucet provides test tokens with no real value.

Faucet address: `faucet0000000000000000000000000000000001`

Default faucet allocation: 1000 DSN per request

## Genesis Configuration

The sandbox uses `deploy/sandbox/genesis.json` for chain initialization:

```bash
# Use custom genesis
dsn init --genesis ./deploy/sandbox/genesis.json --data-dir ~/.dsn-sandbox
```

## Known Limitations

See SANDBOX_KNOWN_LIMITATIONS.md in the DSN repository.

- No real economic value - test tokens only
- Network may be restarted without notice
- Limited storage capacity
- 4-validator consensus (less fault-tolerant than mainnet)

## Support

- GitHub Issues: https://github.com/dsn/dsn/issues
- Discord: https://discord.gg/dsn-network

## File Structure

```
deploy/sandbox/
├── genesis.json        # Sandbox genesis configuration
├── docker-compose.yml  # Local Docker-based sandbox cluster
└── README.md           # This file
```