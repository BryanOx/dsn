# DSN Quick Start Guide

This guide walks you through setting up a local DSN network from scratch. By the end, you'll have a 3-validator network running and be able to submit transactions.

## Prerequisites

- **Go 1.21+** — [Install Go](https://go.dev/doc/install)
- **make** — Available on macOS/Linux; on Windows use [WSL](https://docs.microsoft.com/en-us/windows/wsl/) or Chocolatey
- **git** — For cloning the repository

## Step 1: Clone and Build

```bash
# Clone the repository
git clone https://github.com/dsn/dsn.git
cd dsn

# Build the DSN binary
go build -o dsn ./cmd/dsn

# Verify the build
./dsn --version
```

## Step 2: Generate Genesis Configuration

Generate a genesis file with initial validators and accounts:

```bash
# Generate genesis with 3 validators (default)
./dsn genesis generate --validators 3 --chain-id testnet

# This creates genesis.json with:
# - 3 validator accounts (each with 10000 DSN stake)
# - 1 faucet account (with 1000000 DSN for testing)
```

The genesis command creates:
- Validator key files in `.keys/validator-{0,1,2}/`
- A wallet file for the faucet in `.keys/faucet/wallet.json`
- A `genesis.json` file defining the initial chain state

## Step 3: Start a Single Node

For testing purposes, start a single validator node:

```bash
# Start node with RPC server on port 8545
./dsn node start \
  --genesis ./genesis.json \
  --wallet .keys/faucet/wallet.json \
  --rpc-port 8545 \
  --p2p-port 26656 \
  --data-dir ./data/node0
```

The node will:
1. Initialize the state (create SMT)
2. Set up the mempool
3. Start the P2P network listener
4. Begin block production (if validator)
5. Start the JSON-RPC server

Expected output:
```
INFO Starting DSN node...
INFO State root: <32-byte hash>
INFO RPC server: http://localhost:8545
INFO P2P ID: <peer ID>
INFO Validator: true
INFO Block height: 1
```

## Step 4: Start a 3-Validator Local Cluster

For a proper BFT test network, start 3 validator nodes:

### Terminal 1 — Validator 0 (proposer)

```bash
./dsn node start \
  --genesis ./genesis.json \
  --wallet .keys/validator-0/wallet.json \
  --rpc-port 8545 \
  --p2p-port 26656 \
  --data-dir ./data/node0 \
  --validator-address $(cat .keys/validator-0/address.txt)
```

### Terminal 2 — Validator 1

```bash
./dsn node start \
  --genesis ./genesis.json \
  --wallet .keys/validator-1/wallet.json \
  --rpc-port 8546 \
  --p2p-port 26657 \
  --data-dir ./data/node1 \
  --validator-address $(cat .keys/validator-1/address.txt) \
  --p2p-seed /ip4/127.0.0.1/tcp/26656/p2p/$(cat .keys/validator-0/peerid.txt)
```

### Terminal 3 — Validator 2

```bash
./dsn node start \
  --genesis ./genesis.json \
  --wallet .keys/validator-2/wallet.json \
  --rpc-port 8547 \
  --p2p-port 26658 \
  --data-dir ./data/node2 \
  --validator-address $(cat .keys/validator-2/address.txt) \
  --p2p-seed /ip4/127.0.0.1/tcp/26656/p2p/$(cat .keys/validator-0/peerid.txt)
```

Each validator will:
1. Connect to the seed node (Validator 0)
2. Exchange peer information via P2P gossip
3. Participate in BFT consensus
4. Produce blocks in round-robin fashion

## Step 5: Submit a Transaction

Using cURL or the Go SDK, submit a transfer transaction:

### Via cURL (JSON-RPC)

```bash
# Check faucet balance
curl -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "dsn_getBalance",
    "params": ["<faucet-address>"],
    "id": 1
  }'

# Send a transaction
curl -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "dsn_sendTransaction",
    "params": [{
      "from": "<faucet-address>",
      "to": "<recipient-address>",
      "amount": "100",
      "fee": "1"
    }],
    "id": 1
  }'
```

### Via Go SDK

```go
package main

import (
    "github.com/dsn/dsn/sdk"
)

func main() {
    client := sdk.NewClient("http://localhost:8545")

    // Load wallet
    wallet, _ := sdk.LoadWallet("wallet.json")

    // Send transaction
    tx, err := wallet.SendTransaction(client, sdk.TxParams{
        To:     "recipient-address",
        Amount: "100",
        Fee:    "1",
    })
    if err != nil {
        panic(err)
    }

    println("Transaction sent:", tx.IntentID)
}
```

## Step 6: Check State

Query account state after transaction inclusion:

```bash
# Get account info (balance, nonce, code hash)
curl -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "dsn_getAccount",
    "params": ["<address>"],
    "id": 1
  }'

# Get current block height
curl -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "dsn_blockNumber",
    "params": [],
    "id": 1
  }'

# Get block by number
curl -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "dsn_getBlockByNumber",
    "params": [5],
    "id": 1
  }'
```

## Step 7: Monitor with Prometheus/Grafana

DSN exports Prometheus metrics at `/metrics` endpoint:

```bash
# Enable metrics in node config
# Add to your config.toml:
[metrics]
enabled = true
port = 9090
```

### Set up Prometheus

```yaml
# prometheus.yml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: "dsn-node"
    static_configs:
      - targets: ["localhost:9090"]
```

Run Prometheus:
```bash
docker run -p 9090:9090 -v $(pwd)/prometheus.yml:/etc/prometheus/prometheus.yml prom/prometheus
```

### Grafana Dashboard

Import the DSN dashboard from `charts/dsn/dashboard.json` to visualize:
- Block production rate
- Transaction throughput
- Mempool size
- Validator participation
- State root consistency
- Gas consumption

## Common Commands

```bash
# Check node status
./dsn node status --wallet .keys/validator-0/wallet.json

# Create a new wallet
./dsn wallet create --output ./my-wallet.json

# Check wallet balance
./dsn wallet balance --wallet ./my-wallet.json --rpc-url http://localhost:8545

# Delegate stake to validator
./dsn validator stake \
  --wallet .keys/faucet/wallet.json \
  --validator <validator-address> \
  --amount 1000 \
  --rpc-url http://localhost:8545

# View validator set
./dsn validator list --rpc-url http://localhost:8545
```

## Troubleshooting

### Node won't start
- Check genesis file is valid JSON: `cat genesis.json | jq .`
- Verify wallet file exists and is decrypted
- Ensure P2P ports are not in use

### Transactions not being included
- Check mempool has transactions: `dsn_getPendingTxs`
- Verify your transaction has sufficient fee
- Check node is producing blocks (look at logs)

### Nodes not connecting
- Verify P2P seed addresses are correct
- Check firewall allows TCP on P2P ports
- Use `./dsn node peers` to see connected peers

## Next Steps

- [Architecture Overview](ARCHITECTURE.md) — Deep dive into system design
- [Contract Quick Start](CONTRACT_QUICKSTART.md) — Deploy WASM smart contracts
- [Protocol Spec](docs/dsn_protocol_spec_v_1.md) — Complete technical specification
- [Runbook](RUNBOOK.md) — Production deployment procedures