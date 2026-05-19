# DSN Smart Contracts

This document covers the DSN smart contract system — from quickstart to deep technical details.

---

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

	"github.com/dsn/dsn/sdk/wasm"
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

Contracts have access to 7 host functions:

| Function | Signature | Description |
|----------|-----------|-------------|
| `read_storage` | `(key []byte) ([]byte, error)` | Read a value from contract storage |
| `write_storage` | `(key, value []byte) error` | Write a value to contract storage |
| `emit_event` | `(topic string, data []byte) error` | Emit a deterministic event |
| `read_caller` | `() [20]byte` | Get the address of the transaction sender |
| `read_block_height` | `() uint64` | Get the current block number |
| `read_block_timestamp` | `() uint64` | Get the current block timestamp (unix seconds) |
| `transfer_token` | `(recipient [20]byte, hi uint64, lo uint64) error` | Transfer DSN tokens to an address |

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
- **Host Call Pricing**: Each host function has a fixed gas cost

---

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

---

### Constructor

- **Optional**: Contracts may export an "init" or "constructor" function
- **Called on Deploy**: The constructor is executed once during contract deployment
- **Calldata**: Initialization calldata is passed to the constructor

---

### Deployment

#### Address Derivation

Contract addresses are deterministically derived using:

```
DeriveContractID(sender, nonce, codeHash)
```

Where:
- `sender`: The deployer's address (20 bytes)
- `nonce`: The deployer's transaction nonce
- `codeHash`: SHA-256 hash of the WASM bytecode

This ensures the same contract deployed from the same address with the same nonce always gets the same address.

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
- **Determinism**: All execution must be deterministic — non-deterministic operations will cause consensus failures

---

### Contract Lifecycle

1. **Write**: Develop contract in Go/TypeScript/Rust, compile to WASM
2. **Deploy**: Submit WASM bytecode with optional constructor calldata
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

## Next Steps

- See `docs/EXAMPLES.md` for more complex contract examples (Escrow, Settlement)
- See `docs/SDK_GUIDE.md` for detailed SDK documentation