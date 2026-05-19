# DSN Contract Examples

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

- **Contract Model**: See `CONTRACTS.md` for detailed architecture
- **SDK Guide**: See `SDK_GUIDE.md` for programmatic usage
- **Quickstart**: See `CONTRACT_QUICKSTART.md` for step-by-step tutorial