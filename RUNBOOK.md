# DSN Runbook — CLI & TUI Operations

## 1. CLI Reference

### Wallet

```bash
dsn wallet generate --output wallet.json
dsn wallet sign <tx-file> --key wallet.json       # prints signed JSON to stdout
dsn wallet nonce <address> --rpc http://localhost:8545
```

| Subcommand | Flags | Defaults |
|------------|-------|----------|
| `generate` | `--output` / `-o` | `wallet.json` |
| `sign <file>` | `--key` / `-k` | `wallet.json` |
| `nonce <addr>` | `--rpc` / `-r` | `http://localhost:8545` |

> `wallet sign` reads a simple JSON format (sender, nonce, payload, gasLimit, maxFee) and outputs signed JSON to stdout. For the file-based workflow with auto-numbering, use `tx create` → `tx sign` → `tx send` instead.

### Genesis

```bash
dsn genesis init --devnet --validators 3 --chain-id mynet-1 --output genesis.json
dsn genesis init --validator-addresses <addr1>,<addr2> --chain-id mainnet-1
dsn genesis validate --file genesis.json
dsn genesis inspect --file genesis.json
dsn genesis devnet --validators 3 --output dev/localnet/tmp
```

| Subcommand | Flags | Defaults |
|------------|-------|----------|
| `init` | `--output`, `--chain-id`, `--validators` / `-n`, `--devnet`, `--validator-addresses`, `--allocations` | `genesis.json`, `dsn-localnet-1`, 3 |
| `validate` | `--file` | `genesis.json` |
| `inspect` | `--file` | `genesis.json` |
| `devnet` | `--validators`, `--output` | 3, `dev/localnet/tmp` |

> `genesis init --devnet` creates a single genesis.json with auto-generated validator keys. `genesis devnet` generates a full multi-node directory structure with per-node configs.

> Hash is computed and displayed by `init`, `validate`, and `inspect` — there is no standalone `genesis hash` command.

### Validator

```bash
dsn validator init --output ~/.dsn/validator.key
dsn validator register --key ~/.dsn/validator.key --stake 1000000 --node http://localhost:8545
dsn validator status --key ~/.dsn/validator.key --node http://localhost:8545
```

| Subcommand | Flags | Defaults |
|------------|-------|----------|
| `init` | `--output` | `~/.dsn/validator.key` |
| `register` | `--key`, `--stake`, `--commission`, `--node`, `--rpc` | `~/.dsn/validator.key`, 1000, `http://localhost:8545` |
| `status` | `--key`, `--node`, `--rpc` | `~/.dsn/validator.key`, `http://localhost:8545` |

> Minimum stake is 100,000 DSN. Commission is in basis points (1000 = 10%).

### Transactions

#### tx create

Create an unsigned transaction file with auto-numbering.

```
dsn tx create --sender <address> [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--sender` / `-s` | (required) | Sender address |
| `--to` | `""` | Recipient (for transfer, builds payload automatically) |
| `--amount` / `-a` | `0` | Transfer amount |
| `--nonce` | `0` | Nonce (`0` = auto-resolve from RPC via `dsn_getAccount`) |
| `--rpc` | `http://localhost:8545` | RPC URL for nonce lookup |
| `--output-dir` / `-o` | `transactions` | Output directory for tx files |
| `--gas-limit` | `100000` | Gas limit |
| `--max-fee` | `10` | Max fee per gas |
| `--chain-id` | `0` | Chain ID |
| `--tx-type` | `0` | Transaction type (0=Standard, 1=Deploy, 2=Call, 3=ValidatorReg) |
| `--payload` | `""` | Custom payload (hex, with or without `0x`) |
| `--force` | `false` | Overwrite existing file |

Auto-numbering creates `tx-001.json`, `tx-002.json`, etc. in the output directory. When `--to` and `--amount` are both set, the payload is built automatically (and `--payload` is ignored with a warning).

**Examples:**

```bash
# Simple transfer (builds payload from --to + --amount)
dsn tx create --sender <addr> --to <recipient> --amount 1000

# Custom payload
dsn tx create --sender <addr> --payload 0xdeadbeef

# Explicit nonce, custom directory
dsn tx create --sender <addr> --nonce 5 --output-dir ./my-txs
```

#### tx sign

Sign an unsigned transaction file. Creates a **new** signed file — the original is never modified.

```
dsn tx sign <tx-file> [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--key` / `-k` | `wallet.json` | Wallet key file |
| `--output` / `-o` | auto | Output file (default: `<input>-signed.json`) |

**Examples:**

```bash
# Sign with default wallet → transactions/tx-001-signed.json
dsn tx sign transactions/tx-001.json

# Custom key and output path
dsn tx sign my-tx.json --key ~/keys/wallet.json --output signed.json
```

> If the sender in the tx file does not match the wallet address, a warning is printed but signing proceeds.

#### tx send

Submit a signed transaction to a node via RPC.

```
dsn tx send <signed-tx-file> [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--rpc` | `http://localhost:8545` | RPC URL |

**Example:**

```bash
dsn tx send transactions/tx-001-signed.json --rpc http://localhost:8545
```

> The file must contain a signed transaction (with signature and intentId fields).

### Full workflow example

```bash
# 1. Generate wallet
dsn wallet generate --output wallet.json

# 2. Create a transfer transaction (auto-numbered as transactions/tx-001.json)
dsn tx create --sender <addr> --to <recipient> --amount 1000

# 3. Sign it (creates transactions/tx-001-signed.json)
dsn tx sign transactions/tx-001.json --key wallet.json

# 4. Send it to the node
dsn tx send transactions/tx-001-signed.json --rpc http://localhost:8545
```

---

## 2. TUI Reference

```bash
# Read-only mode (no wallet needed)
dsn-tui --rpc http://localhost:8545

# With wallet (enables signing and sending)
dsn-tui --rpc http://localhost:8545 --wallet wallet.json
```

### Tabs

| Index | Tab | Description |
|-------|-----|-------------|
| 0 | Dashboard | Node status, state root, wallet address |
| 1 | Balance | Check balances |
| 2 | Send | Build, sign, and send DSN to any address |
| 3 | Tx Files | Browse, sign, and send transaction files |
| 4 | Pending | View mempool pending transactions |
| 5 | Wallet | Wallet info |

### Navigation

| Key | Action |
|-----|--------|
| Tab / → | Next tab |
| Shift+Tab / ← | Previous tab |
| r | Refresh current screen (dashboard, pending) |
| q / Ctrl+C | Quit |

### Tx Files Tab

Lists all `tx-*.json` files in the `transactions/` directory. Each entry shows the filename, status (`unsigned` or `SIGNED`), and truncated sender address.

**For unsigned files (wallet must be loaded):**
- Press Enter on a file to view its details
- Press Enter again to confirm signing
- Creates `<filename>-signed.json` in the same directory

**For signed files:**
- Press Enter to view details
- Press Enter again to send to node via RPC

**If no wallet is loaded**, unsigned files can be viewed but not signed. The viewer shows "No wallet loaded. Use --wallet flag to sign."

### Send Tab

Requires a wallet (`--wallet` flag). Enter recipient address and amount, then Enter to confirm, Enter again to send. Builds the transfer payload (20-byte recipient + 8-byte amount BE), signs in memory, and submits via `dsn_sendTransaction`. No files are created.

---

## 3. Transaction File Format

### Unsigned (`transactions/tx-001.json`)

```json
{
  "version": 1,
  "chainId": 0,
  "sender": "16okjApjLUDg6PWZEqoBRXwdbnHoKwLgyr",
  "nonce": 1,
  "payload": "0x",
  "maxFee": 10,
  "gasLimit": 100000,
  "timestamp": 1700000000,
  "txType": 0
}
```

### Signed (`transactions/tx-001-signed.json`)

All unsigned fields plus:

```json
{
  ...same fields...,
  "signature": "0xabcd...",
  "intentId": "0x1234..."
}
```

### `wallet sign` format (simpler)

The `wallet sign` command uses a different, simpler JSON format:

```json
{ "sender": "0x...", "nonce": 0, "payload": "0x...", "gasLimit": 21000, "maxFee": 1000 }
```

Output (to stdout) adds `signature` and `intentId` fields.

---

## 4. Devnet Quick Start

```bash
# Terminal 1 — start devnet
make build
bin/dsn.exe devnet

# Terminal 2 — generate wallet and send a tx
bin/dsn.exe wallet generate --output wallet.json
bin/dsn.exe tx create \
  --sender $(bin/dsn.exe wallet show wallet.json --field address) \
  --to <recipient> \
  --amount 100
bin/dsn.exe tx sign transactions/tx-001.json --key wallet.json
bin/dsn.exe tx send transactions/tx-001-signed.json

# Terminal 3 — TUI (optional)
bin/dsn-tui.exe --rpc http://localhost:8545 --wallet wallet.json
```
