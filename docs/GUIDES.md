# DSN User & Developer Guides

This document merges all user and developer guides for the DSN blockchain.

---

## Table of Contents

- [Quickstart](#quickstart)
- [CLI Reference](#cli-reference)
- [Validator Guide](#validator-guide)
- [Wallet Guide](#wallet-guide)
- [TUI Guide](#tui-guide)
- [SDK Guide](#sdk-guide)
- [Smart Contracts](#smart-contracts)
- [Examples](#examples)

---

## Quickstart

This guide gets you from zero to a running local DSN network in under 15 minutes.

## Prerequisites

- **Go 1.21+** — [Install Go](https://go.dev/doc/install)
- **make** — macOS/Linux; on Windows use [WSL](https://learn.microsoft.com/en-us/windows/wsl/)

## Clone and Build

```bash
git clone <repo-url>
cd dsn

# Option A — go build
go build -o dsn ./cmd/dsn
go build -o dsn-tui ./cmd/dsn-tui

# Option B — make (builds both to bin/ directory)
make build
```

Verify:

```bash
./dsn --help
```

## Primary Path: Devnet (Single Node, In-Memory)

The fastest way to get a running network. Starts an in-memory node with block production, RPC, faucet, and explorer.

```bash
# Terminal 1 — Start devnet
./dsn devnet --indexer
```

This:
- Creates an in-memory blockchain state (temp data dir: `$TMPDIR/dsn-devnet`)
- Generates a pre-funded validator account (1,000,000 DSN)
- Starts block production every 3 seconds
- Starts JSON-RPC on `http://localhost:8545`
- Starts explorer/faucet API on `http://localhost:8080/api/v1`
- Starts Prometheus metrics on `http://localhost:9464/metrics`
- **No contract execution** (VM is nil — deploy/call will fail)

Optional flags: `--rpc-port 8545`, `--explorer-port 8080`, `--reset` (clear indexer DB), `--verbose`.

### Get Tokens from Faucet

```bash
# Terminal 2 — Generate a wallet
./dsn wallet generate -o wallet.json

# Read the address from the output, then request funds:
curl -X POST http://localhost:8080/api/v1/faucet \
  -H "Content-Type: application/json" \
  -d '{"address":"0xYOUR_ADDRESS"}'
```

Faucet dispenses 100 DSN per request with a 60-second rate limit per IP.

### Check Balance

```bash
# Via RPC (named params)
curl -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"dsn_getBalance","params":{"address":"0xYOUR_ADDRESS"},"id":1}'

# Via RPC (positional params)
curl -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"dsn_getBalance","params":["0xYOUR_ADDRESS"],"id":1}'

# Via CLI
./dsn wallet nonce 0xYOUR_ADDRESS --rpc http://localhost:8545
```

### Explore with the TUI

```bash
# Terminal 3 — TUI (read-only or with wallet for sending)
./dsn-tui --rpc http://localhost:8545 --wallet wallet.json
```

The TUI shows a dashboard, balance, pending transactions, and a send screen.

### Send a Transaction

Transactions must be signed offline. Create an unsigned transaction file:

```bash
# tx.json
cat > tx.json << 'EOF'
{
  "sender": "0xYOUR_ADDRESS",
  "nonce": 1,
  "payload": "00000000000000000000000000000000000000000000000000000000000000000000000000000000",  # 20 bytes recipient + 8 bytes amount
  "gasLimit": 21000,
  "maxFee": 1000
}
EOF

# Sign it
./dsn wallet sign tx.json --key wallet.json
```

The output includes `signature` and `intentId`. Submit the signed JSON via RPC:

```bash
curl -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc":"2.0",
    "method":"dsn_sendTransaction",
    "params":{"tx":"{\"sender\":\"0x...\",\"nonce\":1,\"signature\":\"0x...\",\"intentId\":\"0x...\",...}"},
    "id":1
  }'
```

### Check Block Production

```bash
curl -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"dsn_getBlock","params":{"blockNumber":1},"id":1}'
```

## Alternative: Production Node with Genesis

### 1. Generate Genesis

```bash
# Devnet genesis (auto-generates validator keys)
./dsn genesis init --devnet --validators 3 --chain-id dsn-localnet-1 --output genesis.json

# Template genesis (placeholder keys — you supply real keys later)
./dsn genesis init --chain-id my-chain --output genesis.json
```

### 2. Validate

```bash
./dsn genesis validate --file genesis.json
```

### 3. Generate a Validator Key

```bash
./dsn validator init --output ~/.dsn/validator.key
```

### 4. Start the Node

```bash
./dsn node start \
  --genesis genesis.json \
  --validator-key ~/.dsn/validator.key \
  --data-dir ./data
```

The node requires a genesis file and a validator key. It will initialize state from genesis, start the RPC server, and begin block production (if designated as proposer).

### 5. Register Validator (on a live network)

```bash
./dsn validator register \
  --key ~/.dsn/validator.key \
  --stake 100000 \
  --commission 1000 \
  --node http://localhost:8545
```

### 6. Check Status

```bash
./dsn validator status --key ~/.dsn/validator.key --node http://localhost:8545
```

## Multi-Node Devnet (Docker)

```bash
# Generate devnet config with docker-compose files
./dsn genesis devnet --validators 3 --output dev/localnet/tmp

# Start
cd dev/localnet/tmp
docker-compose up -d
```

## Contract Commands

> **Note**: Contract execution requires a VM. The devnet runs in-memory without a VM, so deploy/call will fail on devnet. These commands work against a full production node.

```bash
# Deploy a WASM contract
./dsn contract deploy token.wasm --key wallet.json --rpc http://localhost:8545

# Call a contract (read-only simulation)
./dsn contract call 0xCONTRACT_ADDRESS balanceOf --data 0x... --rpc http://localhost:8545

# Query contract state
./dsn contract query 0xCONTRACT_ADDRESS mykey --rpc http://localhost:8545

# Get transaction receipt
./dsn contract receipt 0xTXHASH --rpc http://localhost:8545
```

## JSON-RPC Reference

| Method | Status | Params |
|--------|--------|--------|
| `dsn_getBlock` | LIVE | `{blockNumber}` or `{blockHash}` (hash requires indexer) |
| `dsn_getTransaction` | LIVE | `{txHash}` |
| `dsn_sendTransaction` | LIVE | `{tx}` (signed JSON) |
| `dsn_getAccount` | LIVE | `{address}` |
| `dsn_getBalance` | LIVE | `{address}` or positional `["0x..."]` |
| `dsn_getContract` | LIVE | `{address}` |
| `dsn_callContract` | LIVE | `{address, entrypoint, data?, gasLimit?}` |
| `dsn_estimateGas` | LIVE | `{address, data?}` |
| `dsn_getContractState` | LIVE | `{address, key}` |
| `dsn_getTransactionReceipt` | LIVE | `{txHash}` (returns pending for mempool txs) |
| `dsn_getSupply` | LIVE | `{}` |
| `dsn_getEvents` | STUB | Requires event indexer (Phase 6) |
| `dsn_getValidators` | STUB | Requires active staking state sync |
| `dsn_getStateRoot` | LEGACY | Returns hardcoded `0x0000...` |
| `dsn_getPendingTxs` | LEGACY | Returns empty array |

RPC endpoint: `POST http://localhost:8545` with `Content-Type: application/json`.

Metrics: `http://localhost:9464/metrics`. Health check: `http://localhost:8545/health`.

## Limitations

- **Devnet has no VM** — contract deploy/call produce errors. Run a production node for contract execution.
- **No event indexer in devnet** — `dsn_getEvents` returns an error.
- **Validator set query** — `dsn_getValidators` requires staking state sync.
- **Multi-node consensus** — `dsn genesis devnet` generates docker-compose files but full BFT requires additional configuration.
- **No wallet balance CLI** — use `dsn_getBalance` RPC directly.

## Troubleshooting

- `file already exists` — genesis init won't overwrite; use `--output` for a different path.
- `genesis file not found` — run `dsn genesis init --devnet` first.
- Devnet state is ephemeral — use `--reset` to clear the indexer DB.
- `method not found` — check the method name against the reference table above.

---

## CLI Reference

> Complete CLI reference generated from command source code. Documents every command, subcommand, flag, and behavior.

---

### Table of Contents

- [Global Persistent Flags](#global-persistent-flags)
- [dsn node start](#dsn-node-start--production-node)
- [dsn genesis](#dsn-genesis--genesis-operations)
- [dsn validator](#dsn-validator--validator-management)
- [dsn wallet](#dsn-wallet--wallet-management)
- [dsn contract](#dsn-contract--smart-contract-operations)
- [dsn devnet](#dsn-devnet--local-development-network)
- [Legacy Mode](#legacy-mode-no-subcommand)
- [RPC Reference](#rpc-reference)
- [Build Reference](#build-reference)

---

## Global Persistent Flags

Apply to all commands.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-v`, `--verbose` | bool | false | Verbose output |
| `-q`, `--quiet` | bool | false | Quiet output (errors only) |
| `--config` | string | `""` | Path to TOML config file |
| `--p2p-port` | int | `0` | P2P port (`0` = disabled) |
| `--p2p-max-peers` | int | `0` | Max P2P peers |
| `--bootstrap-peers` | string | `""` | Comma-separated bootstrap peer addresses |
| `--rpc-port` | int | `0` | RPC port |
| `--metrics-port` | int | `0` | Prometheus metrics port |
| `--data-dir` | string | `""` | Data directory |
| `--chain-id` | uint32 | `0` | Chain ID |
| `--mempool-size` | int | `0` | Mempool max size |
| `--indexer` | bool | false | Enable block indexer |
| `--fast-sync` | bool | false | Enable fast sync mode |
| `--log-level` | string | `""` | Log level (`debug` / `info` / `warn` / `error`) |

Config hierarchy: **CLI flags > env vars (`DSN_*`) > TOML file > built-in defaults**

---

## `dsn node start` — Production Node

Start a full production DSN node with P2P, RPC, and consensus.

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--config` | string | `""` | TOML config file path |
| `--genesis` | string | `""` | Genesis file path **(required)** |
| `--validator-key` | string | `""` | Validator key file path |
| `--data-dir` | string | `""` | Data directory (overrides config) |

### Startup Sequence (14 phases)

1. **Load & validate config** — merge CLI flags, env vars, TOML file, defaults
2. **Create data directory** — ensure it exists
3. **Load genesis file** — validates `ChainID`, `GenesisTime`
4. **Validate genesis** — 8 validation rules
5. **Hash genesis** — verify against stored hash on restart
6. **Init BoltDB** — open KV store
7. **Init genesis state** (fresh start) or load existing state
8. **Load validator registry** — verify validator key
9. **Init consensus engine** — copies parameters
10. **Init networking** — only if P2P enabled
11. **Init RPC** — announces port
12. **Init metrics server** — `/metrics`, `/health` endpoints
13. **Enter sync mode** — `Normal` or `FastSync`
14. **Transition to live** — start consensus or wait

---

## `dsn genesis` — Genesis Operations

### `dsn genesis init`

Create a genesis file. Two modes: **devnet** (auto-generates keys) and **template**.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--output` | string | `"genesis.json"` | Output file path |
| `--chain-id` | string | `"dsn-localnet-1"` | Chain ID |
| `--validators` | int | `3` | Validator count (devnet mode) |
| `--devnet` | bool | false | Auto-generate validator keys |

**Devnet mode**: generates N Ed25519 validator keys, each with 100M stake, treasury at `0x0001` with 1B tokens.

**Template mode**: creates file with placeholder all-zero validator keys (not usable for real networks).

> ⚠ **Dead flags** — declared but not used: `--validator-addresses`, `--allocations`

#### Example

```bash
# Devnet mode
dsn genesis init --devnet --validators 4 --output mynet/genesis.json

# Template mode
dsn genesis init --output template.json
```

### `dsn genesis validate`

Validate a genesis file.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--file` | string | `"genesis.json"` | Genesis file to validate |

Validates: `ChainID`, genesis time, 8 validation rules, prints hash.

#### Example

```bash
dsn genesis validate --file genesis.json
```

### `dsn genesis devnet`

Create a multi-validator local devnet directory structure.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--validators` | int | `3` | Number of validators |
| `--output` | string | `"dev/localnet/tmp"` | Output directory |

Creates node directories with keys, `config.toml`, `genesis.json` — Docker-ready.

#### Example

```bash
dsn genesis devnet --validators 4 --output ./devnet-data
```

---

## `dsn validator` — Validator Management

### `dsn validator init`

Generate a validator Ed25519 key pair.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--output` | string | `"~/.dsn/validator.key"` | Key output path |

Prompts for overwrite confirmation if file exists.

#### Example

```bash
dsn validator init --output ~/.dsn/validator.key
```

### `dsn validator register`

Register a validator with the network.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--key` | string | `"~/.dsn/validator.key"` | Validator key file |
| `--stake` | uint64 | `0` | Stake in DSN (min 100000) |
| `--commission` | uint64 | `1000` | Commission rate (0–10000 = 0–100%) |
| `--node` | string | `"http://localhost:8545"` | Node RPC URL |
| `--rpc` | string | `"http://localhost:8545"` | Alias for `--node` |

> ⚠ **Known issue**: Uses RPC method `dsn_submitTransaction` but the server method is `dsn_sendTransaction`.

#### Example

```bash
dsn validator register \
  --key ~/.dsn/validator.key \
  --stake 500000 \
  --commission 500
```

### `dsn validator status`

Show validator registration status.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--key` | string | `"~/.dsn/validator.key"` | Validator key file |
| `--node` | string | `"http://localhost:8545"` | Node RPC URL |

Aliases: `info`

> ⚠ **Known issue**: Queries RPC method `dsn_getValidator` which does **not exist** on the server.

#### Example

```bash
dsn validator status --key ~/.dsn/validator.key
```

---

## `dsn wallet` — Wallet Management

### `dsn wallet generate`

Generate a new Ed25519 wallet key pair.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-o`, `--output` | string | `"wallet.json"` | Output key file |

Creates JSON with `public_key` and `private_key` hex fields. Address is first 20 bytes of pubkey.

#### Example

```bash
dsn wallet generate -o my-wallet.json
```

### `dsn wallet sign <tx-file>`

Sign an unsigned transaction from a JSON file. Takes exactly 1 argument.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-k`, `--key` | string | `"wallet.json"` | Wallet key file |

#### Input JSON format

```json
{
  "sender": "0x...",
  "nonce": 1,
  "payload": "hex",
  "gasLimit": 50000,
  "maxFee": 1000
}
```

#### Output

Original fields plus `signature` (hex) and `intentId` (hex).

#### Example

```bash
dsn wallet sign tx.json -k my-wallet.json
```

### `dsn wallet nonce <address>`

Get current nonce for an address via RPC. Takes exactly 1 argument (hex address).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-r`, `--rpc` | string | `"http://localhost:8545"` | RPC URL |

#### Example

```bash
dsn wallet nonce 0xabcd1234... -r http://localhost:8545
```

---

## `dsn contract` — Smart Contract Operations

### `dsn contract deploy <wasm-path>`

Deploy a WASM smart contract. Takes exactly 1 argument.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-k`, `--key` | string | `"wallet.json"` | Wallet key file |
| `--gas-limit` | uint64 | `1000000` | Gas limit |
| `-r`, `--rpc` | string | `"http://localhost:8545"` | RPC URL |

Contract address derivation: `SHA256(sender || nonce || codeHash)[:20]`

> ⚠ **Dead flag**: `--abi` declared but never used.

#### Example

```bash
dsn contract deploy my-contract.wasm -k wallet.json --gas-limit 2000000
```

### `dsn contract call <address> <entrypoint>`

Call a contract method locally (read-only). Takes exactly 2 arguments.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--data` | string | `""` | Call data (hex) |
| `--gas-limit` | uint64 | `1000000` | Gas limit |
| `-r`, `--rpc` | string | `"http://localhost:8545"` | RPC URL |

#### Example

```bash
dsn contract call 0xabcd... my_function --data 0xdeadbeef
```

### `dsn contract estimate <address> <entrypoint>`

Estimate gas for a contract call. Takes exactly 2 arguments.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--data` | string | `""` | Call data (hex) |
| `-r`, `--rpc` | string | `"http://localhost:8545"` | RPC URL |

#### Example

```bash
dsn contract estimate 0xabcd... my_function --data 0xdeadbeef
```

### `dsn contract query <address> <key>`

Query contract storage. Takes exactly 2 arguments.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-r`, `--rpc` | string | `"http://localhost:8545"` | RPC URL |

#### Example

```bash
dsn contract query 0xabcd... 0x01
```

### `dsn contract receipt <txid>`

Get a transaction receipt. Takes exactly 1 argument.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-r`, `--rpc` | string | `"http://localhost:8545"` | RPC URL |

#### Example

```bash
dsn contract receipt 0x1234...
```

---

## `dsn devnet` — Local Development Network

Start a single-node devnet with block producer, faucet, explorer, and metrics.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--rpc-port` | int | `8545` | RPC port |
| `--explorer-port` | int | `8080` | Explorer port |
| `--indexer` | bool | false | Enable block indexer |
| `--reset` | bool | false | Reset devnet state |
| `-v`, `--verbose` | bool | false | Verbose output |

### Features

- **In-memory node** — temp directory: `$TMPDIR/dsn-devnet`
- **3-second block producer ticker**
- **Built-in faucet**: `POST /api/v1/faucet {"address":"0x..."}` → 100 DSN, 60s rate limit
- **Explorer**: `http://localhost:8080`
- **Metrics**: `http://localhost:9464/metrics`
- **Max 100 txs per block**

> ⚠ **Limitation**: No contract execution — VM is `nil`.

#### Example

```bash
dsn devnet --rpc-port 8545 --explorer-port 8080 --indexer --reset
```

---

## Legacy Mode (no subcommand)

Running `dsn` without a subcommand uses legacy flag-based mode.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-wallet` | string | `"validator_key.json"` | Wallet key path |
| `-rpc` | string | `":8545"` | RPC address |
| `-genesis` | bool | false | Create genesis account (first run) |

---

## RPC Reference

### Live Methods

| Method | Parameters | Returns |
|--------|------------|---------|
| `dsn_getBlock` | `{blockNumber?, blockHash?}` | `Block` (hash lookup requires indexer) |
| `dsn_getTransaction` | `{txHash}` | `Transaction` (mempool only) |
| `dsn_sendTransaction` | `{tx: json_string}` | `{txHash}` |
| `dsn_getAccount` | `{address}` | `{nonce, balance, codeHash, storageRoot}` |
| `dsn_getBalance` | `{address}` | `{balance: string}` |
| `dsn_getContract` | `{address}` | Contract info |
| `dsn_callContract` | `{address, entrypoint, data?, gasLimit?}` | `{success, result, gasUsed}` |
| `dsn_estimateGas` | `{address, data}` | `{gas: number}` |
| `dsn_getContractState` | `{address, key}` | `{value: hex}` |
| `dsn_getTransactionReceipt` | `{txHash}` | Receipt or pending status |
| `dsn_getSupply` | (none) | Total supply info |

### Stub Methods

| Method | Behavior |
|--------|----------|
| `dsn_getEvents` | Returns `"requires event indexer"` |
| `dsn_getValidators` | Returns `"requires active staking state synchronization"` |
| `dsn_getStateRoot` | Returns hardcoded `0x0000` |
| `dsn_getPendingTxs` | Returns empty array |

### WebSocket Events

- `newHeads`
- `logs`
- `pendingTransactions`

---

## Build Reference

| Command | Description |
|---------|-------------|
| `make build` | Build `bin/dsn` + `bin/dsn-tui` |
| `make test` | Run `go test ./... -cover` |
| `make test-integration` | Run integration tests |
| `make lint` | Run `go vet` + `staticcheck` |
| `make clean` | Remove `bin/` and `dist/` |
| `make build-all` | Cross-compile (linux/amd64, linux/arm64, darwin/amd64, windows/amd64) |
| `make docker-build` | Build Docker image |

---

## Validator Guide

This guide covers validator setup, operation, and maintenance for the DSN blockchain. Validators produce blocks, vote on consensus, and earn rewards proportional to their stake.

> **Status**: Implementation reference. Some CLI commands have known RPC compatibility issues — see [Known Issues](#known-issues-and-workarounds).

---

## 1. Prerequisites

### Build from Source

```bash
git clone <repo-url> dsn
cd dsn
go build -o dsn ./cmd/dsn
```

Requires Go 1.22+.

### Node Access

You need access to a running DSN node with the JSON-RPC endpoint exposed (default: `http://localhost:8545`). The node does not need to be a validator itself — you can register from a separate machine.

---

## 2. Validator Key Generation

Every validator needs an Ed25519 key pair.

### Generate a Key

```bash
./dsn validator init --output ~/.dsn/validator.key
```

If `--output` is omitted, defaults to `~/.dsn/validator.key` (cross-platform: `$HOME` on Linux/macOS, `%USERPROFILE%` on Windows). If the file already exists, you are prompted for overwrite confirmation (y/N).

### Key File Format

The key file is a plain hex file with three lines (permissions `0600`):

```
<private_key_64_bytes_hex>    ← 128 hex chars
<public_key_32_bytes_hex>     ← 64 hex chars
<address_20_bytes_hex>        ← 40 hex chars (no 0x prefix)
```

### Address Derivation

- **Validator address** = `SHA256(publicKey)[:20]` (first 20 bytes of SHA-256 of the raw public key)
- This is **different** from wallet addresses (which use `pubKey[:20]` directly).

### Key Validation

`LoadValidatorKey` performs integrity checks:
- Derives public key from private key and verifies it matches line 2
- Computes address from public key and verifies it matches line 3

### Output on Generation

```
Address:     0x<40_hex_chars>
Public Key:  0x<64_hex_chars>
Key File:    ~/.dsn/validator.key
```

**Backup this file immediately. It cannot be recovered.**

---

## 3. Genesis Setup

### Devnet Genesis with Validators

```bash
./dsn genesis init --devnet --validators 3 --output genesis.json
```

This generates N Ed25519 key pairs automatically and embeds them in the genesis file. Each validator gets:
- **Stake**: 100,000,000,000,000 (100M tokens)
- **Balance**: 1,000,000 (1M tokens)
- **Commission**: 10% (1000 basis points)

### Manual Genesis Configuration

For production, generate a template and manually configure validators in `genesis.json`:

```json
{
  "initial_validators": [
    {
      "address": "0x...",
      "pub_key": "0x...",
      "consensus_key": "0x...",
      "stake": 1000000000000000,
      "commission": "1000"
    }
  ]
}
```

Validators listed in `genesis.json` are automatically registered at chain init via `InitGenesisState` — no separate registration transaction needed.

### Devnet Single Validator

```bash
./dsn devnet
```

This starts a single-validator devnet with:
- Auto-generated key
- 1,000,000 DSN balance
- Block production every 3 seconds
- In-memory mode (no consensus — single node)

---

## 4. Starting a Validator Node

```bash
./dsn node start \
  --genesis genesis.json \
  --validator-key ~/.dsn/validator.key \
  --data-dir ./data
```

| Flag | Description |
|------|-------------|
| `--genesis` | Path to genesis JSON file |
| `--validator-key` | Path to the validator key file |
| `--data-dir` | Directory for chain data (state DB, blocks) |

The node loads the validator key, initializes the state from genesis, and begins participating in block production if the validator is in the active set.

---

## 5. Registration & Staking

If your validator is **not** in the genesis file, register after the chain is running:

```bash
./dsn validator register \
  --key ~/.dsn/validator.key \
  --stake 100000 \
  --commission 1000 \
  --node http://localhost:8545
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--key` | `~/.dsn/validator.key` | Path to validator key |
| `--stake` | — | Stake amount (uint64, minimum 100,000) |
| `--commission` | `1000` | Commission rate in basis points (0–10000, default 1000 = 10%) |
| `--node` / `--rpc` | `http://localhost:8545` | Node RPC endpoint |

### Registration Flow

1. Load validator key from file
2. Fetch nonce via RPC method `dsn_getAccount` (defaults to 1 on error)
3. Build a `ValidatorRegistrationMsg`
4. Encode message as bytes
5. Create a `TxTypeValidatorRegistration` transaction
6. Sign with Ed25519 key
7. Submit via RPC

### Hardcoded Transaction Parameters

| Parameter | Value |
|-----------|-------|
| ChainID | 1 |
| GasLimit | 50,000 |
| MaxFee | 1,000 |

> **⚠ Note**: ChainID is hardcoded to 1. If your devnet uses ChainID 0, the signature may be invalid. See [Known Issues](#known-issues-and-workarounds).

### Staking Model

The `Validator` struct tracks:

| Field | Description |
|-------|-------------|
| Address | Validator address (20 bytes) |
| PublicKey | Ed25519 public key (32 bytes) |
| Stake | Staked amount |
| Commission | Commission rate in basis points |
| Status | Active, Pending, Jailed, or Inactive |

### Minimum Stake

Configured in genesis. Default minimum varies by network profile (devnet, localnet).

---

## 6. Validator Lifecycle

```
Genesis/Register → Pending → Active → Jailed → Inactive
                        ↑         |         |
                        └─────────┘         |
                      (epoch transition)    |
                                            ↓
                                      Auto-unjail
                                     (after cooldown)
```

### Pending → Active

After registration, the validator is in **Pending** status. At the next **epoch transition** (`ProcessEpochTransition`), pending validators are evaluated and activated if they meet the minimum stake and the active set has capacity (`MaxValidators`).

### Active

Active validators:
- Propose blocks when selected as leader
- Vote on consensus rounds
- Earn rewards distributed at the end of each block via `DistributeRewards`
- Are tracked by the `ConsensusState` for block production

### Jailing

A validator is jailed for protocol violations (double-sign, equivocation):
- **Evidence submission**: An `Evidence` struct with validator address, type, height, and proof is submitted
- **Slashing**: Part of the stake is forfeited
- **Jailing**: `JailedUntil` = current height + (epochs × blocks per epoch × 2)
- **Effects**: Cannot propose blocks, does not earn rewards

### Unjailing

`ProcessEpochTransition` checks `JailedUntil` for each jailed validator. If the current block height exceeds `JailedUntil`, the validator is automatically unjailed (status reverts to active).

### Inactive

A validator becomes inactive if it voluntarily stops or fails to meet minimum requirements.

---

## 7. Epoch System

Validators transition between states at epoch boundaries.

| Parameter | Description |
|-----------|-------------|
| `BlocksPerEpoch` | Blocks per epoch (configurable in genesis) |
| `UnstakeCooldownEpochs` | Blocks before unstaking takes effect |
| `MaxValidators` | Maximum active validators |
| `MinimumStake` | Minimum stake to register |

### Default Values

| Network | BlocksPerEpoch |
|---------|----------------|
| devnet | 100 |
| localnet | 10 |

### Epoch Transition (`ProcessEpochTransition`)

Called during `FinalizeBlock`. At each epoch boundary:
1. Activates pending validators (up to `MaxValidators`)
2. Unjails validators whose `JailedUntil` has passed
3. Processes queued unstaking requests

---

## 8. Rewards & Slashing

### Block Reward Distribution

Rewards are distributed at the end of each block via `DistributeRewards`:

| Recipient | Share |
|-----------|-------|
| Validators (proportional to stake) | 70% |
| Burned | 20% |
| Treasury | 10% |

Validator rewards are split proportionally by stake. A validator with 10% of total stake receives 10% of the 70% validator pool.

### Slashing

Slashing is **evidence-based** — requires submission of proof:

- **Double-sign**: Signing two different blocks at the same height
- **Equivocation**: Voting for conflicting proposals

When evidence is submitted:
1. Validator is slashed (a portion of the stake is removed)
2. Status set to `Jailed`
3. `JailedUntil` = current height + (epochs × blocks per epoch × 2)

> **⚠ Note**: There is no automated evidence detection. Evidence must be submitted manually. See [Known Issues](#known-issues-and-workarounds).

---

## 9. Monitoring Validator Status

### Via CLI

```bash
./dsn validator status --key ~/.dsn/validator.key --node http://localhost:8545
```

This loads the local key and prints the address and public key, then queries the RPC method `dsn_getValidator`.

> **⚠ Known issue**: The RPC method `dsn_getValidator` does **not** exist on the server. The status command will fail at the RPC query step.

### Via Direct RPC

Query the account nonce and balance:

```bash
curl -s -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"dsn_getAccount","params":["0x<validator_address>"],"id":1}'
```

Check the current block height:

```bash
curl -s -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
```

### Logs

The node outputs validator activity at startup:

```
INFO[0000] Validator key loaded                          address=0x... pubkey=0x...
INFO[0000] Starting DSN node...
```

---

## 10. Multi-Node Setup

### Using `genesis devnet` (Docker Compose)

```bash
# Generate multi-node devnet structure
./dsn genesis devnet --validators 3

# Deploy with Docker Compose
cd dev/localnet
docker-compose up -d
```

This generates:
- Validator keys for each node
- Genesis file with all validators pre-configured
- Docker Compose configuration for the full network

### Manual Production Setup

1. **Build the binary**:
   ```bash
   go build -o dsn ./cmd/dsn
   ```

2. **Generate genesis** (or prepare manually):
   ```bash
   ./dsn genesis init --devnet --validators 1 --output genesis.json
   ```

3. **Generate validator key on each node**:
   ```bash
   ./dsn validator init --output ~/.dsn/validator.key
   ```

4. **Start each node**:
   ```bash
   ./dsn node start \
     --genesis /path/to/genesis.json \
     --validator-key ~/.dsn/validator.key \
     --data-dir ./data
   ```

5. **Register** (if not in genesis validators):
   ```bash
   ./dsn validator register --key ~/.dsn/validator.key --stake 100000 --commission 1000
   ```

6. **Wait for epoch transition** — the validator becomes active at the next epoch boundary.

---

## 11. Known Issues and Workarounds

| # | Issue | Root Cause | Workaround |
|---|-------|------------|------------|
| 1 | `validator register` fails silently | Uses RPC method `dsn_submitTransaction` but server exposes `dsn_sendTransaction` | Patch `internal/rpc/client.go` to use `dsn_sendTransaction`, or wait for fix |
| 2 | `validator status` fails at RPC call | Calls non-existent RPC method `dsn_getValidator` | Use `dsn_getAccount` directly via curl to check account state |
| 3 | Registration fails on devnet | `ChainID` hardcoded to `1` in registration tx; devnet may use `ChainID=0` | Patch the ChainID constant in the registration code to match your network |
| 4 | No delegation | Delegation system not implemented | Validators must self-stake |
| 5 | No on-chain governance | Governance module not implemented | Configuration changes require network coordination |
| 6 | Slashing requires manual evidence | No automated equivocation detection | Implement external monitoring and submit evidence programmatically |
| 7 | Pending status not observable | `isPending` hardcoded to `false` (TODO in code) | Cannot distinguish pending vs inactive via CLI |
| 8 | Devnet validators have no separate key files | Keys embedded in genesis.json | Extract keys from genesis if needed for backup |

---

## 12. Operational Best Practices

### Key Management

- Store the validator key with `0600` permissions (enforced by the CLI)
- Back up the key file to an offline, encrypted location
- Never share the private key
- Use a dedicated machine — do not run other services on the validator node

### Epoch Awareness

- Registration takes effect only at epoch boundaries
- Monitor `BlocksPerEpoch` to predict transitions
- Plan maintenance between epoch transitions

### Reward Collection

- Rewards are distributed automatically per block
- 70% of block rewards go to validators proportionally by stake
- No manual claim needed

### Security

- Keep the node binary updated
- Monitor disk space on the data directory
- Restrict RPC access to trusted networks (bind to localhost or use a firewall)
- Do not expose the validator key file over any network

### Updates

```bash
git pull origin main
go build -o dsn ./cmd/dsn
# Stop, replace binary, restart
```

For multi-validator networks, coordinate upgrades to avoid losing consensus quorum.

---

## Wallet Guide

> **Scope**: User-facing wallet CLI, transaction signing, key management, and manual RPC queries.
> For **validator-specific** key operations, see [Validator Guide](#validator-guide).

---

## 1. Overview

The DSN wallet system manages Ed25519 key pairs for the DSN blockchain. Two distinct key types exist:

| Key Type | File Format | Address Derivation | Primary Use |
|----------|-------------|-------------------|-------------|
| Wallet Key | JSON (`wallet.json`) | First 20 bytes of public key | Signing transactions, receiving funds |
| Validator Key | Three-line hex (`validator.key`) | `SHA256(pubkey)[:20]` | Validator identity, consensus participation |

Both use Ed25519 but differ in address derivation and storage format.

---

## 2. Key Types

### 2.1 Wallet Key

Defined in `wallet/wallet.go`. Ed25519 key pair stored as JSON with `0600` permissions.

```json
{
  "public_key": "a1b2c3...",
  "private_key": "deadbeef..."
}
```

**Methods**:
- `GenerateKey()` — creates a new Ed25519 key pair
- `Sign(data)` — signs arbitrary bytes
- `SignHash(hash)` — signs a 32-byte hash
- `SaveKey(path)` — writes JSON with 0600 permissions
- `LoadKey(path)` — reads and validates JSON key file

**Address**: first 20 bytes of the raw Ed25519 public key.

### 2.2 Validator Key

Defined in `wallet/validator.go`. Different derivation from wallet keys.

**File format** (three lines of hex):
```
<private_key>   ← 128 hex chars (64 bytes)
<public_key>    ← 64 hex chars (32 bytes)
<address>       ← 40 hex chars (20 bytes)
```

**Validation** on load:
- Public key must match private key
- Address must match `SHA256(pubkey)[:20]`

> **Important**: Validator addresses are DIFFERENT from wallet addresses for the same key material because of the SHA256 derivation step.

---

## 3. CLI Commands

All wallet operations are under the `dsn wallet` subcommand.

### 3.1 Generate Wallet

```bash
dsn wallet generate --output my-wallet.json
```

Flags:
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--output` | `-o` | `wallet.json` | Output path for the key file |

**Expected output**:
```
Address: 0xabcdef0123456789abcdef0123456789abcdef01
Public Key: a1b2c3d4e5f6...
Key saved to: my-wallet.json
```

### 3.2 Sign Transaction

```bash
dsn wallet sign tx.json --key my-wallet.json
```

Flags:
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--key` | `-k` | `wallet.json` | Path to wallet key file |

**Input format** (`tx.json`):
```json
{
  "sender": "0xabcdef0123456789abcdef0123456789abcdef01",
  "nonce": 1,
  "payload": "hex-encoded-bytes",
  "gasLimit": 50000,
  "maxFee": 1000
}
```

**Output**: the original fields plus:
```json
{
  "...": "...",
  "signature": "deadbeef...",
  "intentId": "a1b2c3d4..."
}
```

**Signing process**:
1. Compute `IntentID = SHA256(sender || nonce || payload)`
2. Sign the 32-byte IntentID with Ed25519 wallet key
3. Return signature + intentId alongside original fields

For a standard transfer, see [Transaction Types & Payload Formats](#51-standard-transfer-payload-txtypestandard-0).

### 3.3 Get Nonce

```bash
dsn wallet nonce 0xabcdef0123456789abcdef0123456789abcdef01 --rpc http://localhost:8545
```

Flags:
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--rpc` | `-r` | `http://localhost:8545` | RPC endpoint URL |

**What it does**: Calls the `dsn_getAccount` JSON-RPC method and prints the full account result.

**Expected output**:
```json
{
  "address": "0x...",
  "nonce": 0,
  "balance": "0x...",
  "codeHash": "0x..."
}
```

---

## 4. Transaction Signing Workflow

Step-by-step process to create and sign a transaction.

### Step 1: Generate a Wallet Key

```bash
dsn wallet generate -o my-wallet.json
```

Save the address output — it's your "from" address.

### Step 2: Get Current Nonce

```bash
dsn wallet nonce 0x<your-address>
```

### Step 3: Prepare Unsigned Transaction

Create `unsigned-tx.json`:

```json
{
  "sender": "0x<your-address>",
  "nonce": <nonce-from-step-2>,
  "payload": "<hex-encoded-payload>",
  "gasLimit": 50000,
  "maxFee": 1000
}
```

### Step 4: Sign It

```bash
dsn wallet sign unsigned-tx.json -k my-wallet.json
```

Save the output (now contains `signature` and `intentId`).

### Step 5: Submit via RPC

Send the signed transaction to the network:

```bash
curl -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "dsn_sendTransaction",
    "params": { "signedTx": { ... } },
    "id": 1
  }'
```

---

## 5. Transaction Types & Payload Formats

### Transaction Structure

| Field | Type | Description |
|-------|------|-------------|
| `Version` | `uint8` | Transaction format version |
| `ChainID` | `uint32` | Chain identifier |
| `TxType` | `uint8` | `0`=Standard, `1`=ValidatorRegistration, `2`=DeployContract |
| `Sender` | `Address` (20 bytes) | Sender address |
| `Nonce` | `uint64` | Sender nonce |
| `Payload` | `[]byte` | Transaction data (type-dependent) |
| `GasLimit` | `uint64` | Maximum gas units |
| `MaxFee` | `uint64` | Maximum fee per gas unit |
| `Signature` | `[64]byte` | Ed25519 signature |
| `IntentID` | `[32]byte` | `SHA256(sender || nonce || payload)` |

### 5.1 Standard Transfer Payload (`TxTypeStandard = 0`)

```
| 20 bytes recipient address | 8 bytes amount (big-endian) |
```

**Total**: 28 bytes, hex-encoded for the `payload` field.

Example payload for sending 1000 tokens to `0xabcd...01`:

```bash
# recipient address (20 bytes) + amount 1000 as uint64 BE
payload="abcdef0123456789abcdef0123456789abcdef0100000000000003e8"
```

### 5.2 Validator Registration (`TxTypeValidatorRegistration = 1`)

Used when a validator registers on-chain. Payload includes the validator's public key and proof. See [Validator Guide](#validator-guide) for details.

### 5.3 Deploy Contract (`TxTypeDeployContract = 2`)

WASM bytecode as the payload. Used for smart contract deployment.

---

## 6. Manual RPC Usage

The CLI has no `dsn wallet balance` command. Use `curl` directly.

### Get Balance

```bash
curl -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "dsn_getBalance",
    "params": { "address": "0xabcdef0123456789abcdef0123456789abcdef01" },
    "id": 1
  }'
```

### Get Account (includes nonce + balance + code hash)

```bash
curl -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "dsn_getAccount",
    "params": { "address": "0xabcdef0123456789abcdef0123456789abcdef01" },
    "id": 1
  }'
```

### Send Signed Transaction

```bash
curl -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "dsn_sendTransaction",
    "params": {
      "version": 0,
      "chainId": 0,
      "txType": 0,
      "sender": "0x...",
      "nonce": 1,
      "payload": "0x...",
      "gasLimit": 50000,
      "maxFee": 1000,
      "signature": "0x...",
      "intentId": "0x..."
    },
    "id": 1
  }'
```

---

## 7. Key Security Recommendations

| Practice | Detail |
|----------|--------|
| **File permissions** | Always `0600` (owner read/write only). The `SaveKey()` method enforces this. |
| **Different keys per environment** | Never reuse devnet keys on testnet/mainnet. |
| **Ephemeral devnet keys** | Devnet auto-generates keys in a temp directory — treat them as disposable. |
| **Private key isolation** | Never paste private keys into terminals, logs, or shared documents. |
| **Overwrite protection** | The `dsn validator init` command prompts before overwriting. Manual file removal is needed for wallet keys — no overwrite prompt exists there. |

---

## 8. Backup Best Practices

### Wallet Key (wallet.json)

1. After `dsn wallet generate`, immediately copy the key file to offline storage.
2. Store on encrypted media (USB drive, hardware wallet).
3. Keep the address handy — it's not derivable from memory if you lose the file.

### Recovery Risk

Without the key file, there is **no recovery mechanism**. No mnemonic, no seed phrase. Loss of `wallet.json` means permanent loss of access.

### Backup Checklist

- [ ] Copy `wallet.json` to encrypted USB drive
- [ ] Record the 0x address separately (printed during generation)
- [ ] Store a second backup in a different physical location
- [ ] Test key loading on a separate machine with `LoadKey()`

---

## 9. Address Format

All DSN addresses are:

- **20 bytes** (40 hex chars) with `0x` prefix
- **Case-insensitive** — `0xABCD` and `0xabcd` refer to the same address
- Displayed with `0x` prefix in all CLI output and RPC parameters

**Example**:
```
0xabcdef0123456789abcdef0123456789abcdef01
```

---

## TUI Guide

**Version**: v0.1.0-sandbox
**Binary**: `dsn-tui.exe` (Windows) / `dsn-tui` (Linux/macOS)
**Source**: `cmd/dsn-tui/`
**Tech Stack**: [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea), [bubbles](https://github.com/charmbracelet/bubbles), [lipgloss](https://github.com/charmbracelet/lipgloss)

---

## 1. Overview

`dsn-tui` is a terminal-based user interface for interacting with a DSN node. It provides a visual dashboard for checking state, querying balances, sending transactions, inspecting the mempool, and viewing wallet info — all without leaving the terminal.

It connects to any DSN node via JSON-RPC and optionally loads a local wallet for signing transactions.

---

## 2. Installation

`dsn-tui` is built alongside the rest of DSN. From the project root:

```bash
go build -o dsn-tui ./cmd/dsn-tui/
```

On Windows this produces `dsn-tui.exe`; on Linux/macOS it produces `dsn-tui`.

No additional dependencies are required beyond the Go toolchain and the DSN module dependencies.

---

## 3. Launch Options

```bash
./dsn-tui --wallet wallet.json --rpc http://localhost:8545
```

### Flags

| Flag        | Description                              | Default                    |
|-------------|------------------------------------------|----------------------------|
| `--wallet`  | Path to wallet JSON file (optional)     | *none* (read-only mode)    |
| `--rpc`     | RPC endpoint URL                         | `http://localhost:8545`    |

- If `--wallet` is omitted, the TUI launches in **read-only mode** — tabs that require a wallet (Send, Wallet) show limited or no data.
- The TUI uses the **alternate screen buffer** — your terminal returns to its previous state on quit.

---

## 4. Tab-by-Tab Guide

Navigation between tabs uses **Tab** / **Shift+Tab** or the number keys **1–5**.

---

### 4.1 Dashboard (Tab 1)

The main landing tab showing node and wallet summary.

| Field          | Source                                     | Notes                                  |
|----------------|--------------------------------------------|----------------------------------------|
| State Root     | `dsn_getStateRoot` RPC                     | **Stub** — always returns `0x0000...`  |
| Wallet Address | Loaded from `--wallet`                     | Displayed only if wallet loaded        |
| Balance        | `dsn_getBalance` RPC                       | Displayed only if wallet loaded        |
| Nonce          | `dsn_getNonce` RPC                         | Displayed only if wallet loaded        |
| RPC Status     | Connection check on startup                | Shows connected / disconnected         |

- Data is fetched automatically when the tab loads.
- Press **`r`** to manually refresh all displayed values.

---

### 4.2 Balance (Tab 2)

Query the balance of any address.

- Text input field: enter an Ethereum-style address (`0x...`).
- Press **Enter** to submit the query via `dsn_getBalance`.
- The result is displayed below the input field.
- Press **Esc** to clear the input and return to the tab view.

---

### 4.3 Send (Tab 3)

Build and sign a transaction, then submit it to the network.

- **Recipient**: text input for the destination address.
- **Amount**: text input for the value to send.
- Press **Enter** on the submit button to proceed.
- A confirmation dialog shows the transaction details before sending.
- Confirming builds an unsigned transaction with:
  - Hardcoded `nonce = 1`
  - Hardcoded `maxFee = 10`
  - Payload = raw recipient address bytes (not properly encoded as address + amount)
- The transaction is signed with the loaded wallet key and submitted via `dsn_sendTransaction`.

> **⚠️ Known issues**: See [Limitations](#7-known-limitations-and-workarounds).

---

### 4.4 Pending (Tab 4)

View pending mempool transactions.

- Fetches via `dsn_getPendingTxs` RPC.
- **Stub** — the RPC always returns an empty array.
- The tab always displays **"No pending transactions"** regardless of actual mempool state.

---

### 4.5 Wallet (Tab 5)

View loaded wallet information.

- Displays the wallet address and balance.
- If no wallet was loaded via `--wallet`, shows a read-only placeholder message.
- All data is read-only in this view.

---

## 5. Keyboard Shortcuts

| Key            | Action                  |
|----------------|-------------------------|
| `Tab`          | Next tab                |
| `Shift+Tab`    | Previous tab            |
| `1`–`5`        | Jump to tab 1–5         |
| `r`            | Refresh current view    |
| `Enter`        | Submit form / confirm   |
| `Esc`          | Cancel / go back        |
| `q` / `Ctrl+C` | Quit the TUI            |

---

## 6. Transaction Flow Walkthrough

1. Launch with a wallet: `./dsn-tui --wallet wallet.json`
2. Press **`2`** to jump to the Balance tab — verify the sender address has funds.
3. Press **`3`** to jump to the Send tab.
4. Enter the recipient address and amount.
5. Press **Enter** on the submit button.
6. Review the confirmation dialog and confirm.
7. If the RPC returns success, the transaction was submitted.
8. Press **`4`** to check the Pending tab — note that it will always show empty (stub limitation).
9. Use the CLI (`dsn tx pool`) or check the node logs to verify the transaction was received.

> **Workaround**: If the send fails, use `dsn wallet nonce <address>` via the CLI to check the actual nonce, then note that the TUI currently hardcodes nonce to `1`.

---

## 7. Known Limitations and Workarounds

These are documented, real limitations of the v0.1.0-sandbox TUI. No feature described below exists but is missing — these are conscious trade-offs or stubs.

| # | Limitation | Detail | Workaround |
|---|-----------|--------|------------|
| 1 | **Dashboard state root stub** | `dsn_getStateRoot` always returns `0x0000...0000` | Use CLI command `dsn state root` to get the real state root |
| 2 | **Pending tab always empty** | `dsn_getPendingTxs` is a stub returning `[]` | Use `dsn tx pool` CLI command to inspect the mempool |
| 3 | **Hardcoded nonce = 1** | Send tab always sets nonce to 1, which fails if the sender's actual nonce is > 1 | Run `dsn wallet nonce <address>` via CLI, then manually adjust (workaround requires code change) |
| 4 | **Incorrect transaction payload** | The send payload is just the recipient address bytes, not properly encoding address + amount | The RPC may reject or mishandle the transaction; use CLI `dsn tx send` for correct encoding |
| 5 | **No auto-refresh** | All views are static after initial load — press `r` manually | Use `r` key to refresh, or rely on CLI for real-time monitoring |
| 6 | **Address truncation** | Addresses displayed in the UI are truncated to 16 characters | Use `dsn wallet info` CLI command to see full addresses |
| 7 | **Potential data races** | Error handling uses goroutines that may race on state updates | Avoid rapid repeated `r` presses; if the UI freezes, restart the TUI |
| 8 | **No contract interaction tab** | Contracts cannot be called or deployed from the TUI | Use CLI `dsn tx send --data` or a separate tool like Cast |
| 9 | **No validator monitoring tab** | No validator set or staking information available | Use CLI commands under `dsn validator` |
| 10 | **No WebSocket subscriptions** | The TUI uses HTTP polling only, no real-time event subscriptions | Use `dsn events` CLI command or connect directly via WebSocket |

---

## 8. Troubleshooting

### RPC connection fails

- Verify the node is running: `curl http://localhost:8545 -X POST -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'`
- Check the `--rpc` flag points to the correct URL and port.
- Ensure the node's RPC server is not bound to `127.0.0.1` if connecting remotely.

### Wallet not loading

- Confirm the wallet JSON file exists at the path specified by `--wallet`.
- Validate the wallet file format — it should match the output of `dsn wallet create`.
- Run `dsn wallet info wallet.json` to test loading outside the TUI.

### Balance shows 0

- If on a devnet, run the faucet first: `dsn faucet <address>`.
- Verify the address is correct — addresses are checksummed and hex-encoded.
- Confirm the node is synced and connected to peers.

### Send fails

- Check the sender's actual nonce: `dsn wallet nonce <address>`. The TUI hardcodes nonce=1 (see [Limitation #3](#7-known-limitations-and-workarounds)).
- Verify the sender has sufficient balance.
- Check the node logs for RPC error details.
- As a fallback, send via CLI: `dsn tx send --from <sender> --to <recipient> --amount <value>`.

### UI freezes or behaves unexpectedly

- Avoid spamming the `r` refresh key rapidly (see [Limitation #7](#7-known-limitations-and-workarounds)).
- Restart the TUI if the display becomes corrupt — it uses the alternate screen buffer and restores cleanly on restart.

---

## 9. Cross-References

- **[Wallet Guide](#wallet-guide)** — Wallet creation, import, and management.
- **[CLI Reference](#cli-reference)** — Full CLI command reference including `dsn tx send`, `dsn wallet nonce`, `dsn state root`, and `dsn tx pool`.

---

## SDK Guide

## Go SDK (`sdk/`)

The Go SDK provides a full-featured client for interacting with the DSN network.

### Installation

```bash
import "github.com/BryanOx/dsn/sdk"
```

### Initialization

```go
client := dsn.New("http://localhost:8080")
```

### Block Methods

```go
// Get a specific block by number and hash
block, err := client.GetBlock(ctx, 1234, "0xabc123...")

// Get the latest block
latest, err := client.GetLatestBlock(ctx)
```

### Transaction Methods

```go
// Get a transaction by hash
tx, err := client.GetTransaction(ctx, "0xdef456...")

// Send a signed transaction
txHash, err := client.SendTransaction(ctx, signedTx)

// Send a raw transaction (hex-encoded)
txHash, err := client.SendRawTransaction(ctx, "0xabc123...")
```

### Account Methods

```go
// Get account state (nonce, balance, code hash)
account, err := client.GetAccount(ctx, "0x1234...")

// Get account balance
balance, err := client.GetBalance(ctx, "0x1234...")
```

### Contract Methods

```go
// Get contract info
contract, err := client.GetContract(ctx, "0xabcd...")

// Get contract bytecode
code, err := client.GetContractCode(ctx, "0xabcd...")

// Call a contract (read-only simulation)
result, err := client.CallContract(ctx, "0xabcd...", []byte{0x01, 0x02}, 100000)

// Estimate gas for a call
gas, err := client.EstimateGas(ctx, "0xabcd...", []byte{0x01})
```

### Event Methods

```go
// Query events with filters
events, err := client.GetEvents(ctx, dsn.EventFilter{
    Contract: "0xabcd...",
    FromBlock: 1000,
    ToBlock: 2000,
    Topics: []string{"Transfer"},
})
```

### Validator Methods

```go
// Get validators for a specific epoch
validators, err := client.GetValidators(ctx, 100)

// Get current active validators
current, err := client.GetCurrentValidators(ctx)

// Get total token supply
supply, err := client.GetSupply(ctx)
```

### Options

```go
// Custom HTTP client
client := dsn.New("http://localhost:8080", dsn.WithHTTPClient(httpClient))

// Custom timeout
client := dsn.New("http://localhost:8080", dsn.WithTimeout(30 * time.Second))

// Retry configuration
client := dsn.New("http://localhost:8080", dsn.WithRetry(3, 100 * time.Millisecond))
```

### TxBuilder (Fluent Transaction Building)

```go
tx := client.TxBuilder().
    From("0x1234...").
    To("0xabcd...").
    Data([]byte{0x01}).
    GasLimit(100000).
    Build()
```

### Wallet (Key Management)

```go
// Generate a new wallet
wallet := dsn.GenerateWallet()

// Import from private key
wallet := dsn.ImportWallet(privateKeyHex)

// Sign a transaction
signed := wallet.SignTransaction(tx)
```

### WebSocket Subscriptions

```go
// Subscribe to new blocks
ch := client.SubscribeBlocks(ctx)
for block := range ch {
    fmt.Println(block.Number)
}

// Subscribe to events
ch := client.SubscribeEvents(ctx, "0xabcd...")
for event := range ch {
    fmt.Println(event.Topic, event.Data)
}
```

---

## TypeScript SDK (`sdks/dsn-js/`)

The TypeScript SDK provides a browser-compatible client for DSN.

### Installation

```bash
npm install dsn-js
```

### Initialization

```typescript
import { DSNClient } from 'dsn-js';

const client = new DSNClient('http://localhost:8080');
```

### Block Methods

```typescript
// Get current block number
const blockNumber = await client.getBlockNumber();

// Get block by number or hash
const block = await client.getBlock({ number: 1234 });
const blockByHash = await client.getBlock({ hash: '0xabc...' });
```

### Transaction Methods

```typescript
// Get transaction
const tx = await client.getTransaction('0xdef...');

// Get transaction receipt
const receipt = await client.getTransactionReceipt('0xdef...');

// Send raw transaction (signed hex)
const txHash = await client.sendRawTransaction('0xabc123...');

// Send transaction (from wallet)
const txHash = await client.sendTransaction({
    to: '0xabcd...',
    data: '0x0102',
    gasLimit: 100000,
    from: '0x1234...'  // or uses default wallet
});
```

### Account Methods

```typescript
// Get account state
const account = await client.getAccount('0x1234...');
// Returns: { nonce, balance, codeHash }

// Get balance
const balance = await client.getBalance('0x1234...');

// Get nonce
const nonce = await client.getNonce('0x1234...');
```

### Contract Methods

```typescript
// Deploy a contract
const result = await client.deployContract({
    sender: '0x1234...',
    bytecode: '0x...',  // hex-encoded WASM
    maxFee: 1000000,
    gasLimit: 200000
});
// Returns: { contractAddress, transactionHash }

// Call a contract (read-only)
const result = await client.callContract({
    contract: '0xabcd...',
    data: '0x010203',
    sender: '0x1234...'  // optional, uses default wallet
});

// Estimate gas
const gasEstimate = await client.estimateGas({
    contract: '0xabcd...',
    data: '0x010203',
    sender: '0x1234...'  // optional
});
```

### Event Methods

```typescript
// Query events
const events = await client.getEvents({
    contract: '0xabcd...',
    fromBlock: 1000,
    toBlock: 2000,
    topics: ['Transfer']
});
```

### Network Methods

```typescript
// Get current validators
const validators = await client.getValidators();

// Get total supply
const supply = await client.getSupply();

// Health check
const healthy = await client.health();
```

---

## Go WASM SDK (`sdk/wasm/`)

The WASM SDK is used for **writing contracts** in Go. It provides helper functions to interact with the DSN runtime.

### Installation

```go
import "github.com/BryanOx/dsn/sdk/wasm"
```

### Build Requirements

```bash
GOOS=wasip1 GOARCH=wasm go build -o contract.wasm ./contract.go
```

### Core Functions

#### Caller & Context

```go
// Get the address of the transaction sender
caller := wasm.ReadCaller() // Returns [20]byte

// Get current block height
blockHeight := wasm.ReadBlockHeight() // Returns uint64

// Get current block timestamp (unix seconds)
timestamp := wasm.ReadBlockTimestamp() // Returns uint64
```

#### Storage

```go
// Read from contract storage
value, err := wasm.ReadStorage(key []byte)

// Write to contract storage
err := wasm.WriteStorage(key, value []byte)
```

#### Events

```go
// Emit a deterministic event
err := wasm.EmitEvent(topic string, data []byte)
```

#### Calldata

```go
// Get the calldata passed to the entrypoint
calldata := wasm.ReadCalldata() // Returns []byte

// Set calldata for the next entrypoint call (for init)
wasm.SetCalldataFromMemory(ptr uint32, length uint32)

// Read arguments sequentially from calldata
arg := wasm.ReadArg(size int) // Returns []byte
```

#### Encoding Helpers

```go
// Big-endian conversions
num := wasm.BEToUint64(bytes []byte)      // []byte -> uint64
bytes := wasm.Uint64ToBE(num uint64)      // uint64 -> []byte
```

#### Token Transfers

```go
// Transfer tokens (128-bit amount)
err := wasm.Transfer(recipient [20]byte, hi uint64, lo uint64)

// Transfer tokens (64-bit amount)
err := wasm.TransferUint64(recipient [20]byte, amount uint64)
```

### Example Contract Structure

```go
package main

import (
    "github.com/BryanOx/dsn/sdk/wasm"
)

func main() {}

// export init
func initWrapper() int64 {
    // Constructor code (optional)
    return 0
}

func transfer() int64 {
    // Parse calldata: recipient (20 bytes) + amount (8 bytes)
    recipient := wasm.ReadCaller() // or parse from calldata
    amount := wasm.ReadArg(8)
    amount64 := wasm.BEToUint64(amount)

    // Read balance from storage
    balanceKey := append([]byte("balance:"), recipient[:]...)
    balanceBytes, _ := wasm.ReadStorage(balanceKey)
    balance := wasm.BEToUint64(balanceBytes)

    // Transfer
    if balance >= amount64 {
        wasm.TransferUint64(recipient, amount64)
        wasm.EmitEvent("Transfer", append(recipient[:], wasm.Uint64ToBE(amount64)...))
    }

    return 0
}

//export transfer
func transferWrapper() int64 {
    return transfer()
}

// We need main for wasip1
func main() {}
```

---

## Additional Resources

- **CLI Tools**: Use `dsn` CLI for quick interactions (`dsn contract deploy`, `dsn contract call`)
- **Examples**: See `examples/` directory for complete contract examples
- **Quickstart**: See [Smart Contracts](#smart-contracts) section for a step-by-step guide
- **API Reference**: See [Smart Contracts](#smart-contracts) section for detailed contract system documentation

---

## Smart Contracts

This document covers the DSN smart contract system — from quickstart to deep technical details.

---

> ⚠ **Known Issues**: This document contains inaccuracies versus the current implementation. `transfer_token` is a STUB (does not transfer), the contract constructor receives NO arguments, events cannot be queried post-execution (`dsn_getEvents` is a stub), and the devnet does not execute contracts. See inline notes for details.

## Part 1: Quickstart — Write, Deploy, and Interact

### Prerequisites

- **Go 1.21+**: Required for WASM compilation with `wasip1` target
- **DSN Node**: A running DSN node (see `README.md` for setup)
- **DSN CLI**: Install with `go install ./cmd/dsn`

### Step 1: Write a Contract

Create a simple token contract (`token.go`):

```go
package main

import (
	"encoding/binary"

	"github.com/BryanOx/dsn/sdk/wasm"
)

// State keys
var (
	balancePrefix   = []byte("balance:")
	totalSupplyKey = []byte("totalSupply")
)

func main() {}

// init is the constructor - called on deployment
//export init
func initWrapper() int64 {
	calldata := wasm.ReadCalldata()
	if len(calldata) < 8 {
		return 1 // Error: need total supply
	}

	totalSupply := binary.BigEndian.Uint64(calldata[:8])
	wasm.WriteStorage(totalSupplyKey, wasm.Uint64ToBE(totalSupply))

	// Set initial balance for deployer
	caller := wasm.ReadCaller()
	balanceKey := append(balancePrefix, caller[:]...)
	wasm.WriteStorage(balanceKey, wasm.Uint64ToBE(totalSupply))

	return 0
}

// balanceOf returns the balance of an address
//export balanceOf
func balanceOfWrapper() int64 {
	// Read 20-byte address from calldata
	addr := wasm.ReadArg(20)
	balanceKey := append(balancePrefix, addr...)

	value, err := wasm.ReadStorage(balanceKey)
	if err != nil || len(value) == 0 {
		return 0
	}

	// Return as uint64 (caller expects big-endian)
	return int64(binary.BigEndian.Uint64(value))
}

// transfer transfers tokens to another address
//export transfer
func transferWrapper() int64 {
	calldata := wasm.ReadCalldata()
	if len(calldata) < 28 {
		return 1 // Error: need 20 bytes recipient + 8 bytes amount
	}

	recipient := calldata[:20]
	amount := binary.BigEndian.Uint64(calldata[20:28])

	// Get caller balance
	caller := wasm.ReadCaller()
	fromKey := append(balancePrefix, caller[:]...)
	fromBalanceBytes, _ := wasm.ReadStorage(fromKey)
	fromBalance := binary.BigEndian.Uint64(fromBalanceBytes)

	if fromBalance < amount {
		return 1 // Error: insufficient balance
	}

	// Deduct from sender
	wasm.WriteStorage(fromKey, wasm.Uint64ToBE(fromBalance-amount))

	// Add to recipient
	toKey := append(balancePrefix, recipient...)
	toBalanceBytes, _ := wasm.ReadStorage(toKey)
	toBalance := binary.BigEndian.Uint64(toBalanceBytes)
	wasm.WriteStorage(toKey, wasm.Uint64ToBE(toBalance+amount))

	// Emit Transfer event
	eventData := make([]byte, 28)
	copy(eventData[:20], recipient)
	copy(eventData[20:], wasm.Uint64ToBE(amount))
	wasm.EmitEvent("Transfer", eventData)

	return 0
}

// totalSupply returns the total token supply
//export totalSupply
func totalSupplyWrapper() int64 {
	value, err := wasm.ReadStorage(totalSupplyKey)
	if err != nil || len(value) == 0 {
		return 0
	}
	return int64(binary.BigEndian.Uint64(value))
}
```

### Step 2: Compile

Compile the Go contract to WASM:

```bash
GOOS=wasip1 GOARCH=wasm go build -o token.wasm ./token.go
```

This produces `token.wasm` — your deployable contract bytecode.

### Step 3: Start a Node

Start a local DSN node in development mode:

```bash
dsn node start --dev
```

This will:
- Start a local blockchain
- Create a genesis block
- Enable API server (default: `http://localhost:8080`)

### Step 4: Deploy

Deploy your compiled contract:

```bash
dsn contract deploy token.wasm
```

Expected output:
```
Contract deployed successfully!
Contract Address: 0xabc123def456...
Transaction Hash: 0x789abcdef012...
```

**Note the contract address** — you'll need it for subsequent calls.

### Step 5: Check Balance

Query the balance of your address:

```bash
dsn contract call <contract-address> balanceOf --data 0x<your-address-as-hex>
```

Example:
```bash
dsn contract call 0xabc123... balanceOf --data 0xabc1234567890abcdef1234567890abcdef12
```

Expected output:
```json
{
  "data": "0x03e8",
  "gasUsed": 1234
}
```

### Step 6: Transfer

Transfer tokens to another address:

```bash
dsn contract call <contract-address> transfer --data 0x<recipient-address-20-bytes><amount-8-bytes>
```

Example:
```bash
# Transfer 1000 tokens (0x03E8 = 1000 in big-endian)
dsn contract call 0xabc123... transfer --data 0xdef4567890123456789012345678901234567e803000000000000
```

Expected output:
```json
{
  "data": "0x00",
  "gasUsed": 5000
}
```

The `0x00` return value indicates success.

### Step 7: Query State

Query the total supply:

```bash
dsn contract query <contract-address> total_supply
```

Query another account's balance:

```bash
dsn contract call <contract-address> balanceOf --data 0x<address-hex>
```

### Step 8: Estimate Gas

Before making a call, estimate the gas required:

```bash
dsn contract estimate <contract-address> transfer --data 0x<calldata>
```

Example:
```bash
dsn contract estimate 0xabc123... transfer --data 0xdef4567890123456789012345678901234567e803000000000000
```

Expected output:
```json
{
  "gasEstimate": 5000
}
```

---

## Part 2: Contract Model & Lifecycle

### Contract Model

The DSN smart contract system is built on **WASM-based** execution using the [wazero](https://github.com/tetratelabs/wazero) runtime.

#### Key Characteristics

- **Deterministic Execution**: Uses `CoreFeaturesV1` specification to ensure deterministic behavior across all nodes
- **No WASI**: Contracts run in a sandboxed environment without WASI syscalls — only host functions are available
- **Memory Limit**: Maximum 256 pages (16 MiB) per contract

#### Contract Requirements

A valid DSN contract must:
1. Export at least one function (either an entrypoint or constructor/init)
2. Conform to the WASM MVP specification
3. Use only the available host functions

---

### Host Functions

Contracts have access to 7 host functions imported from the `"env"` WASM module:

| Function | WASM Parameters | Returns | Gas Cost | Description |
|----------|---------------|---------|----------|-------------|
| `read_storage` | `(contractID_ptr, contractID_len, key_ptr, key_len, out_ptr)` | `error_code` | 20 | Read a value from contract storage |
| `write_storage` | `(contractID_ptr, contractID_len, key_ptr, key_len, value_ptr, value_len)` | `error_code` | 50 + 2/byte | Write a value (max 64 KiB) |
| `emit_event` | `(topic_ptr, topic_len, data_ptr, data_len)` | `void` | 30 | Emit a deterministic event |
| `read_caller` | `(out_ptr)` | `error_code` | 10 | Get transaction sender (writes 20 bytes) |
| `read_block_height` | `(none)` | `uint64` | 10 | Get current block number |
| `read_block_timestamp` | `(none)` | `uint64` | 10 | Get block timestamp (unix seconds) |
| `transfer_token` | `(recipient_ptr, recipient_len, amount_hi, amount_lo)` | `error_code` | 100 | ⚠ **STUB**: always returns success (0), does NOT transfer |

> The Go SDK (`sdk/wasm/`) wraps these into typed functions (`wasm.ReadStorage`, `wasm.EmitEvent`, etc.). The WASM-level interface uses raw pointer+length pairs; the SDK manages memory layout automatically.

---

### Execution Model

#### Option B: Context Timeout + Host Call Gas Metering

- **Timeout**: Maximum 30 seconds per contract execution
- **Gas Metering**: Gas is consumed per host function call, not per WASM opcode
- **Rationale**: Pure WASM loops can run up to 30 seconds before timeout — no per-opcode metering is performed

#### Gas Model

- **Pre-execution Reservation**: Gas must be reserved before execution begins
- **Refund on Success**: Unused gas is refunded to the caller
- **No Refund on Revert**: If execution reverts, all gas is consumed
- **Host Call Pricing**: Each host function has a fixed gas cost (see [Host Functions](#host-functions) table)

#### Gas Schedule

| Operation | Cost |
|-----------|------|
| Base WASM op | 1 |
| Memory op | 3 |
| Control flow | 5 |
| Non-deterministic op | 100 |
| Deploy base | 1000 |
| Per code byte (deploy) | 1 |

**Gas Limits**:
- Max gas per call: 10,000,000
- Default deploy gas: 1,000,000
- Max contract code size: 1 MiB

---

### Memory Model

- **ContractID** at fixed WASM memory offset **1024** — written by the VM before calling the entrypoint
- **Calldata** at offset **0** — entrypoint argument bytes
- **SDK buffer zones**: caller at offset 32, result at 64, temp at 256
- Host functions use pointer+length parameters, not ABI-encoded structs
- Contracts self-parse calldata — no fixed ABI specification

### Storage

- **Backend**: Sparse Merkle Tree (SMT)
- **Namespacing**: Each contract has its own storage namespace (keys are hashed with SHA-256)
- **Key Size Limit**: Maximum 256 bytes per storage key
- **Value Size Limit**: Maximum 64 KiB per storage value
- **Key Derivation**: `SHA-256(contractID || key)` — ensures no key collisions between contracts

---

### Events

- **Deterministic Ordering**: Events are emitted in the order they are called
- **Block Scope**: Events are scoped to the current block
- **Header Inclusion**: Events are aggregated into an `EventsRoot` included in the block header
- **Limit**: Maximum 1024 events per transaction
- **Data Limit**: Maximum 64 KiB per event data

#### Event Structure

```
Topic: string (max 128 bytes)
Data: []byte (max 64 KiB)
```

> ⚠ Events accumulated during execution are discarded if the transaction reverts. The `dsn_getEvents` RPC endpoint is a **STUB** — events **cannot** be queried after execution. This will be implemented in a future release.

---

### Constructor

- **Optional**: Contracts may export an "init" or "constructor" function
- **Called on Deploy**: The constructor is executed once during contract deployment
- **⚠ No Arguments**: The constructor is called WITHOUT arguments — `ConstructorArgs` deployment parameters are ignored. Contracts that need initialization data must use a separate setup entrypoint or read from storage.

---

### Deployment

#### Address Derivation

Contract addresses are deterministically derived using:

```
contractID = SHA-256(deployer || nonce || codeHash)[:20]
```

Where:
- `deployer`: The deployer's address (20 bytes)
- `nonce`: The deployer's transaction nonce
- `codeHash`: SHA-256 hash of the WASM bytecode

The result is a 20-byte contract address, deterministically derived.

---

### Limitations

| Limitation | Value |
|------------|-------|
| Execution Timeout | 30 seconds |
| Memory | 256 pages (16 MiB) |
| Events per Transaction | 1024 |
| Event Data Size | 64 KiB |
| Storage Value Size | 64 KiB |
| Storage Key Size | 256 bytes |

#### Important Notes

- **Pure WASM Loops**: Loops that don't call host functions can run up to 30 seconds before timeout
- **No Per-Opcode Metering**: Gas is only consumed on host function calls
- **Devnet Limitation**: Devnet mode (`--dev`) does NOT execute contracts — the VM is nil. Use a real network node for contract interactions.
- **Determinism**: All execution must be deterministic — non-deterministic operations will cause consensus failures

---

### Contract Lifecycle

1. **Write**: Develop contract in Go/TypeScript/Rust, compile to WASM
2. **Deploy**: Submit WASM bytecode
3. **Execute**: Call contract functions via transactions
4. **Query**: Read contract state via view calls (no gas charged)
5. **Events**: Subscribe to emitted events for state changes

---

### Security Considerations

- **Sandboxed Execution**: Contracts cannot access the filesystem or network directly
- **Resource Limits**: Memory and execution time are strictly enforced
- **Deterministic**: All state changes must be reproducible by all validators
- **No External Calls**: Contracts cannot make external HTTP requests or call other contracts directly

---

## Common Workflows

### Using the SDK Programmatically

**Go:**
```go
client := dsn.New("http://localhost:8080")

// Deploy
tx := client.TxBuilder().
    To(nil).  // contract creation
    Data(wasmBytes).
    Build()
signed := wallet.SignTransaction(tx)
txHash, _ := client.SendTransaction(ctx, signed)

// Call
result, _ := client.CallContract(ctx, contractAddr, callData, gasLimit)
```

**TypeScript:**
```typescript
const client = new DSNClient('http://localhost:8080');

// Deploy
const { contractAddress } = await client.deployContract({
    sender: '0x1234...',
    bytecode: '0x...',
    gasLimit: 200000
});

// Call
const result = await client.callContract({
    contract: contractAddress,
    data: '0x...'
});
```

---

## Troubleshooting

### Compilation Errors

- **"unknown platform"**: Ensure `GOOS=wasip1 GOARCH=wasm` is set
- **"undefined: wasm"**: Ensure the WASM SDK is imported correctly

### Deployment Errors

- **"out of gas"**: Increase gas limit or optimize contract
- **"invalid bytecode"**: Verify WASM file is valid

### Runtime Errors

- **"execution reverted"**: Check contract logic and calldata format
- **"timeout"**: Contract exceeded 30-second execution limit

---

## Examples

This document describes the example contracts in the `examples/` directory.

---

## 1. Token Contract

**Location**: `examples/token/main.go`

**Description**: A simple ERC-20-like fungible token contract. Users can deploy with an initial supply, check balances, and transfer tokens.

### Exported Functions

| Function | Parameters | Returns | Description |
|----------|-----------|---------|-------------|
| `init` | `totalSupply` (8-byte BE uint64) | `0` on success, `1` on error | Constructor — sets total supply and credits deployer |
| `balanceOf` | `owner` (20-byte address) | `balance` (8-byte BE uint64) | Returns the token balance of an address |
| `transfer` | `recipient` (20-byte) + `amount` (8-byte BE) | `0` on success, `1` on error | Transfers tokens from caller to recipient |
| `totalSupply` | (none) | `supply` (8-byte BE uint64) | Returns the total token supply |

### Calldata Format

- **init**: First 8 bytes = total supply (big-endian)
  - Example: `0x0000000005f5e100` = 1,000,000 tokens

- **balanceOf**: First 20 bytes = owner address
  - Example: `0x1234567890123456789012345678901234567890`

- **transfer**: First 20 bytes = recipient, next 8 bytes = amount
  - Example: `0x<recipient-20-bytes><amount-8-bytes>`

### Events

**Transfer**
- **Topic**: `"Transfer"`
- **Data**: 28 bytes = recipient (20) + amount (8)
- **Emitted**: On every successful transfer

### CLI Examples

```bash
# Deploy with 1,000,000 supply
dsn contract deploy token.wasm --init 0x0000000005f5e100

# Check balance
dsn contract call 0xabc... balanceOf --data 0x<your-address>

# Transfer 100 tokens
dsn contract call 0xabc... transfer --data 0x<recipient-20><amount-8>
```

---

## 2. Escrow Contract

**Location**: `examples/escrow/main.go`

**Description**: A secure escrow contract where a third-party arbiter holds tokens and can release to a beneficiary or refund the depositor. Supports deposits, conditional releases, and refunds.

### Exported Functions

| Function | Parameters | Returns | Description |
|----------|-----------|---------|-------------|
| `init` | `arbiter` (20-byte address) | `0` on success | Constructor — sets the arbiter address |
| `deposit` | (none — uses caller) | `0` on success, `1` if nothing to transfer | Deposits tokens from caller into escrow |
| `release` | `beneficiary` (20-byte address) | `0` on success, `1` on error | Arbiter releases funds to beneficiary |
| `refund` | (none) | `0` on success, `1` on error | Arbiter refunds deposited tokens to depositor |

### Calldata Format

- **init**: First 20 bytes = arbiter address

- **release**: First 20 bytes = beneficiary address

### Events

**Deposit**
- **Topic**: `"Deposit"`
- **Data**: 20 bytes = depositor address
- **Emitted**: When someone deposits tokens

**Release**
- **Topic**: `"Release"`
- **Data**: 28 bytes = depositor (20) + amount (8)
- **Emitted**: When arbiter releases funds

**Refund**
- **Topic**: `"Refund"`
- **Data**: 20 bytes = depositor address
- **Emitted**: When arbiter refunds

### CLI Examples

```bash
# Deploy with arbiter
dsn contract deploy escrow.wasm --init 0x<arbiter-address-20-bytes>

# Deposit tokens (caller sends tokens with the call)
dsn contract call 0xabc... deposit

# Arbiter releases to beneficiary
dsn contract call 0xabc... release --data 0x<beneficiary-address>

# Arbiter refunds depositor
dsn contract call 0xabc... refund
```

---

## 3. Settlement Contract

**Location**: `examples/settlement/main.go`

**Description**: A settlement contract for multi-party state channel settlements. Parties can commit to a hash and later settle by revealing the preimage and exchanging tokens atomically.

### Exported Functions

| Function | Parameters | Returns | Description |
|----------|-----------|---------|-------------|
| `commit` | `commitmentHash` (32-byte) | `0` on success | Record a commitment hash |
| `settle` | `party1` (20) + `amt1` (8) + `party2` (20) + `amt2` (8) + `nonce` (8) | `0` on success, `1` on error | Execute settlement if valid |

### Calldata Format

- **commit**: First 32 bytes = SHA-256 commitment hash

- **settle**:
  - Bytes 0-19: party1 address
  - Bytes 20-27: party1 amount (BE uint64)
  - Bytes 28-47: party2 address
  - Bytes 48-55: party2 amount (BE uint64)
  - Bytes 56-63: nonce (BE uint64)

### Events

**Commit**
- **Topic**: `"Commit"`
- **Data**: 32 bytes = commitment hash
- **Emitted**: When a party commits

**Settlement**
- **Topic**: `"Settlement"`
- **Data**: 56 bytes = party1 (20) + amount1 (8) + party2 (20) + amount2 (8)
- **Emitted**: When a settlement executes

### CLI Examples

```bash
# Commit to a hash
dsn contract call 0xabc... commit --data 0x<32-byte-hash>

# Settle: Alice pays Bob 100, Bob pays Alice 50
dsn contract call 0xabc... settle --data 0x<alice-20><100><bob-20><50><nonce>
```

---

## 4. Event Subscriber

**Location**: `examples/event-subscriber/main.go`

**Description**: A Go binary (not a WASM contract) that polls and listens to contract events from the DSN network. Useful for monitoring, indexing, and building event-driven applications.

**Type**: Go binary (runs on host, not in WASM)

### Command-Line Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-rpc` | string | `http://localhost:8080` | RPC endpoint URL |
| `-contract` | string | (required) | Contract address to watch |
| `-interval` | int | `5` | Polling interval in seconds |
| `-limit` | int | `100` | Maximum events per query |

### Usage

```bash
# Watch events from a specific contract
./event-subscriber -contract 0xabc123...

# Custom RPC and polling interval
./event-subscriber -rpc http://localhost:9090 -contract 0xabc... -interval 2

# Limit results per query
./event-subscriber -contract 0xabc... -limit 50
```

### Output Format

The subscriber prints events in this format:

```
Block: 1234, Tx: 0xabc..., Event: Transfer, Data: 0x<hex>
Block: 1235, Tx: 0xdef..., Event: Transfer, Data: 0x<hex>
```

### Use Cases

- **Indexing**: Build an event indexer for dApps
- **Monitoring**: Alert on specific event patterns
- **Analytics**: Aggregate event data for dashboards
- **Testing**: Verify events are emitted correctly

---

## Example Workflows

### Deploy Token and Monitor Transfers

```bash
# 1. Deploy token
dsn contract deploy token.wasm --init 0x0000000005f5e100

# 2. Start event subscriber in background
./event-subscriber -contract 0xabc123... &
SUBSCRIBER_PID=$!

# 3. Make transfers
dsn contract call 0xabc... transfer --data 0x<...>

# 4. Kill subscriber when done
kill $SUBSCRIBER_PID
```

### Multi-Party Settlement

```bash
# Party A commits
dsn contract call 0xabc... commit --data 0x<commitment-hash>

# Party B commits (can be different contract or same)
dsn contract call 0xabc... commit --data 0x<different-commitment>

# Arbiter settles
dsn contract call 0xabc... settle --data 0x<party-a><amt-a><party-b><amt-b><nonce>
```

---

## Further Reading

- **Contract Model**: See [Smart Contracts](#smart-contracts) section for detailed architecture
- **SDK Guide**: See [SDK Guide](#sdk-guide) section for programmatic usage