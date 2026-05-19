# DSN SDK Guide

## Go SDK (`sdk/`)

The Go SDK provides a full-featured client for interacting with the DSN network.

### Installation

```bash
import "github.com/dsn/dsn/sdk"
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
import "github.com/dsn/dsn/sdk/wasm"
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
    "github.com/dsn/dsn/sdk/wasm"
)

func init() {
    // Constructor code (optional)
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
- **Quickstart**: See `CONTRACT_QUICKSTART.md` for a step-by-step guide
- **API Reference**: See `CONTRACTS.md` for detailed contract system documentation