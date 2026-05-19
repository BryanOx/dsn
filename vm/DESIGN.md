# Phase 4A — Deterministic WASM Smart Contract VM

## Overview

Add a deterministic WebAssembly (WASM) smart contract execution environment to DSN using [wazero](https://github.com/tetratelabs/wazero), a pure-Go WASM runtime with no CGo dependency and deterministic behavior by default. Contracts are deployed as WASM bytecode, called via new transaction types, and executed during block building. The VM is fully deterministic: same inputs at the same block height always produce identical state roots, events, and gas usage.

---

## 1. Package Structure

```
vm/
├── vm.go          # VM struct, Execute entry point
├── runtime.go     # wazero wrapper, deterministic config
├── gas.go         # Gas schedule, GasMeter
├── errors.go      # VM-specific error vars
├── storage.go     # Contract storage bridge (kvstore → SMT)
├── events.go      # EventLog, Event types
├── host.go        # Host function module definition
└── contract.go    # Deploy helpers, DeriveContractID
```

All packages import `github.com/dsn/dsn/types`, `github.com/dsn/dsn/state`, and `github.com/tetratelabs/wazero`.

---

## 2. Types in `types/` — New Transaction Types

### 2.1 DeployContractTx

```go
// DeployContractTx carries the WASM bytecode and constructor arguments.
// Once deployed, the contract is assigned a deterministic ContractID
// derived from deployer, nonce, and code hash.
type DeployContractTx struct {
    Sender    Address
    Nonce     uint64
    WASMCode  []byte  // max 1 MiB
    ConstructorArgs []byte
    MaxFee    uint64
    GasLimit  uint64
    Timestamp uint64
    Signature []byte
}
```

### 2.2 CallContractTx

```go
// CallContractTx invokes an already-deployed contract by its ContractID.
// The Function field selects which exported WASM function to call.
type CallContractTx struct {
    Sender     Address
    Nonce      uint64
    ContractID Hash    // target contract
    Function   string  // exported WASM function name
    Args       []byte  // ABI-encoded arguments
    MaxFee     uint64
    GasLimit   uint64
    Timestamp  uint64
    Signature  []byte
}
```

### 2.3 ContractMetadata

```go
type ContractMetadata struct {
    ContractID   Hash
    Deployer     Address
    CodeHash     Hash     // SHA-256 of the WASM bytecode
    DeployHeight uint64
    DeployNonce  uint64
}
```

### 2.4 Event

```go
type Event struct {
    ContractID Hash
    Topic      string
    Data       []byte
    BlockHeight uint64
    TxIndex    int
}
```

### 2.5 New Error Vars

```go
var (
    ErrContractCodeTooLarge = errors.New("contract code exceeds 1 MiB limit")
    ErrContractNotFound     = errors.New("contract not found")
    ErrGasLimitExceeded     = errors.New("gas limit exceeded")
    ErrVMExecution          = errors.New("WASM execution failed")
    ErrHostFunction         = errors.New("host function error")
    ErrCallDepthExceeded    = errors.New("call depth exceeds limit (64)")
    ErrContractReverted     = errors.New("contract execution reverted")
    ErrMemoryLimitExceeded  = errors.New("WASM memory exceeds 64 KiB limit")
)
```

---

## 3. VM Core — `vm/vm.go`

### 3.1 VM Struct

```go
package vm

import (
    "context"
    "github.com/dsn/dsn/types"
    "github.com/dsn/dsn/state"
    "github.com/tetratelabs/wazero"
)

type VM struct {
    runtime  *wazero.Runtime
    hasher   types.Hasher
    meter    *GasMeter
    ctx      context.Context
}

func NewVM(ctx context.Context, hasher types.Hasher) (*VM, error)

// Execute runs a contract transaction (deploy or call) against the given state.
// It returns the execution result or an error if the transaction is rejected.
func (vm *VM) Execute(tx interface{}, st state.StateDB) (*ExecutionResult, error)
```

### 3.2 ExecutionResult

```go
type ExecutionResult struct {
    GasUsed    uint64
    ReturnData []byte
    Events     []types.Event
    Reverted   bool
}
```

### 3.3 Execute Logic

```go
func (vm *VM) Execute(tx interface{}, st state.StateDB) (*ExecutionResult, error) {
    switch t := tx.(type) {
    case *types.DeployContractTx:
        return vm.executeDeploy(t, st)
    case *types.CallContractTx:
        return vm.executeCall(t, st)
    default:
        return nil, types.ErrInvalidEncoding
    }
}
```

`executeDeploy`:
1. Validate bytecode size ≤ 1 MiB
2. Hash bytecode → `codeHash`
3. Derive `contractID = DeriveContractID(t.Sender, t.Nonce, codeHash)`
4. Compile WASM module (wazero `CompileModule`) — rejects invalid WASM
5. Store bytecode and metadata in state
6. Call `constructor` WASM export (if present) with constructor args
7. Return result

`executeCall`:
1. Load contract code and metadata from state
2. Create new `GasMeter` with `t.GasLimit`
3. Instantiate WASM module with deterministic config + host functions
4. Call the exported function named `t.Function` with `t.Args`
5. Each host function deducts gas before execution
6. Capture events, return data
7. No external IO — all state mutations go through host functions → SMT

---

## 4. Runtime — `vm/runtime.go`

```go
package vm

import (
    "context"
    "github.com/tetratelabs/wazero"
)

// DeterministicConfig returns a wazero RuntimeConfig that enforces:
//   - No filesystem access
//   - No network access
//   - No environment variables
//   - WASM memory capped at 64 KiB
//   - No random number generation sources
func DeterministicConfig() wazero.RuntimeConfig

// NewRuntime creates a wazero runtime with deterministic config.
func NewRuntime(ctx context.Context) (*wazero.Runtime, error)
```

Key configuration points:

```go
func DeterministicConfig() wazero.RuntimeConfig {
    config := wazero.NewRuntimeConfig().
        // Disable all non-deterministic features
        WithCloseNotifier(false)  // no exit/close notifications
    
    // wazero does not expose env/fs by default — safe
    // Memory limit enforced at module compilation
    return config
}
```

Memory cap: set `wazero.ModuleConfig.WithMemoryLimitPages(4)` (4 pages × 64 KiB = 256 KiB; actual max per contract = 1 page = 64 KiB).

---

## 5. Gas — `vm/gas.go`

### 5.1 GasMeter

```go
type GasMeter struct {
    limit    uint64
    consumed uint64
    mutex    sync.Mutex
}

func NewGasMeter(limit uint64) *GasMeter
func (g *GasMeter) Consume(amount uint64) error          // returns ErrGasLimitExceeded
func (g *GasMeter) Remaining() uint64
func (g *GasMeter) Consumed() uint64
```

### 5.2 Gas Schedule

| Operation | Gas Cost | Notes |
|-----------|----------|-------|
| Per WASM instruction (base) | 1 | Each WASM opcode burned as executed |
| Per WASM instruction (heavy) | 10 | memory.grow, br_table, call_indirect |
| `read_storage(contract_id, key)` | 20 | SMT proof path hash computation |
| `write_storage(contract_id, key, value)` | 50 | SMT insert + recompute |
| `emit_event(topic, data)` | 10 | Append to event log |
| `read_caller()` | 5 | Return sender address |
| `read_block_height()` | 5 | Return current block height |
| `read_block_timestamp()` | 5 | Return block timestamp |
| `transfer_token(recipient, amount)` | 100 | Account balance update + SMT |
| Deployment base cost | 5000 | Fixed overhead |
| Per byte of WASM code (deploy) | 1 | Scaled by code size |
| Per byte of storage value (write) | 2 | Scaled by value length |

```go
const (
    GasPerInstruction      uint64 = 1
    GasPerHeavyInstruction uint64 = 10
    GasReadStorage         uint64 = 20
    GasWriteStorage        uint64 = 50
    GasEmitEvent           uint64 = 10
    GasReadCaller          uint64 = 5
    GasReadBlockHeight     uint64 = 5
    GasReadBlockTimestamp  uint64 = 5
    GasTransferToken       uint64 = 100
    GasDeployBase          uint64 = 5000
    GasPerCodeByte         uint64 = 1
    GasPerStorageByte      uint64 = 2
)
```

### 5.3 Metered WASM Execution

WASM instruction metering is achieved via:

1. **Compile-time counter insertion**: After `wazero.CompileModule`, iterate exported functions and wrap each with an entry-level gas check that calls `meter.Consume(GasPerInstruction * estimatedOps)`.

2. **Fallback**: Use `meter.Consume(1)` per host function call, with a safety timeout context for pure WASM loops.

Design choice: wazero does not expose per-opcode hooks. The practical approach is:
- Count host function calls (they're always metered)
- For pure WASM compute, rely on gas limit + context timeout
- Pre-compute static gas cost at compilation and inject a preamble check

---

## 6. Storage — `vm/storage.go`

### 6.1 ContractStore Interface

```go
// ContractStore is the subset of state.StateDB needed by the VM.
type ContractStore interface {
    GetCode(codeHash types.Hash) ([]byte, error)
    SetCode(codeHash types.Hash, code []byte) error
    GetContractMeta(contractID types.Hash) (*types.ContractMetadata, error)
    SetContractMeta(contractID types.Hash, meta *types.ContractMetadata) error
    GetContractStorage(contractID types.Hash, key []byte) ([]byte, error)
    SetContractStorage(contractID types.Hash, key []byte, value []byte) error
    GetAccount(addr types.Address) (*state.Account, error)
    SetAccount(addr types.Address, acc *state.Account) error
}
```

### 6.2 SMT Key Namespace Layout

Since `InMemoryState.kvstore` (and its SMT counterpart) use string keys, we namespace all contract data:

| SMT Key | Value | Description |
|---------|-------|-------------|
| `code:<hex(contractID)>` | WASM bytecode | Raw compiled WASM |
| `meta:<hex(contractID)>` | gob/JSON-serialized ContractMetadata | Contract deployment info |
| `storage:<hex(contractID)>:<hex(key)>` | arbitrary bytes | Contract persistent storage |

`hex()` = lowercase hex encoding of 32-byte hash, producing 64-char string.

### 6.3 Storage Bridge Implementation

```go
// ContractStoreBridge adapts state.StateDB to ContractStore using kvstore namespacing.
type ContractStoreBridge struct {
    db     state.StateDB
    hasher types.Hasher
}

func NewContractStoreBridge(db state.StateDB, hasher types.Hasher) *ContractStoreBridge

func (b *ContractStoreBridge) keyForCode(contractID types.Hash) string {
    return "code:" + hex.EncodeToString(contractID[:])
}

func (b *ContractStoreBridge) keyForMeta(contractID types.Hash) string {
    return "meta:" + hex.EncodeToString(contractID[:])
}

func (b *ContractStoreBridge) keyForStorage(contractID types.Hash, key []byte) string {
    keyHash, _ := b.hasher.Hash(key)
    return "storage:" + hex.EncodeToString(contractID[:]) + ":" + hex.EncodeToString(keyHash[:])
}
```

All values stored through the bridge are serialized using `gob` or binary encoding (matching the existing `Account.Encode` pattern using `io.Writer`/`io.Reader`).

---

## 7. Host Functions — `vm/host.go`

### 7.1 Host Module Specification

```go
// InstantiateHostModule registers deterministic host functions on the wazero runtime.
// Each host function:
//   1. Deducts gas BEFORE execution via GasMeter
//   2. Performs no external IO
//   3. Returns error to the WASM guest on failure (no panic)
func (vm *VM) InstantiateHostModule(ctx context.Context, moduleConfig wazero.ModuleConfig, meter *GasMeter, env *HostEnv) error
```

### 7.2 HostEnv — Per-Call Context

```go
// HostEnv carries the per-execution context needed by host functions.
type HostEnv struct {
    Caller      types.Address
    ContractID  types.Hash
    BlockHeight uint64
    BlockTime   uint64
    Store       ContractStore
    EventLog    *EventLog
    Meter       *GasMeter
}
```

### 7.3 Host Function Signatures (exported to WASM as `dsn` module)

```go
// read_storage(contract_id_ptr: i32, contract_id_len: i32, key_ptr: i32, key_len: i32, out_ptr: i32) → i32 (0=ok, -1=error)
func hostReadStorage(ctx context.Context, module api.Module, contractIDPtr, contractIDLen, keyPtr, keyLen, outPtr uint32) uint32 {
    // 1. Deduct GasReadStorage from meter
    // 2. Read contract_id from WASM memory
    // 3. Read key from WASM memory
    // 4. Call Store.GetContractStorage(contractID, key)
    // 5. Write result to WASM memory at out_ptr
    // 6. Return 0 on success, -1 on error
}

// write_storage(contract_id_ptr, contract_id_len, key_ptr, key_len, value_ptr, value_len) → i32
func hostWriteStorage(ctx context.Context, module api.Module, contractIDPtr, contractIDLen, keyPtr, keyLen, valuePtr, valueLen uint32) uint32 {
    // 1. Deduct GasWriteStorage + GasPerStorageByte * len(value)
    // 2. Validate value ≤ 64 KiB
    // 3. Write to Store.SetContractStorage
    // 4. Return 0 on success, -1 on error
}

// emit_event(topic_ptr, topic_len, data_ptr, data_len) → i32
func hostEmitEvent(ctx context.Context, module api.Module, topicPtr, topicLen, dataPtr, dataLen uint32) uint32 {
    // 1. Deduct GasEmitEvent
    // 2. Append to EventLog
    // 3. Return 0
}

// read_caller(out_ptr) → i32  (writes 20 bytes to out_ptr)
func hostReadCaller(ctx context.Context, module api.Module, outPtr uint32) uint32 {
    // 1. Deduct GasReadCaller
    // 2. Copy env.Caller bytes to WASM memory at out_ptr
    // 3. Return 0
}

// read_block_height() → i64
func hostReadBlockHeight(ctx context.Context, module api.Module) uint64 {
    // 1. Deduct GasReadBlockHeight
    // 2. Return env.BlockHeight
}

// read_block_timestamp() → i64
func hostReadBlockTimestamp(ctx context.Context, module api.Module) uint64 {
    // 1. Deduct GasReadBlockTimestamp
    // 2. Return env.BlockTime
}

// transfer_token(recipient_ptr, recipient_len, amount_hi, amount_lo) → i32
// amount is 128-bit split into hi (upper 64) and lo (lower 64)
func hostTransferToken(ctx context.Context, module api.Module, recipientPtr, recipientLen, amountHi, amountLo uint32) uint32 {
    // 1. Deduct GasTransferToken
    // 2. Read recipient address from WASM memory
    // 3. Compute amount from hi:lo
    // 4. Lookup/call state.Transfer(env.Caller, recipient, amount)
    // 5. Return 0 on success, -1 on error (e.g. insufficient balance)
}
```

### 7.4 Host Module Registration

```go
func (vm *VM) InstantiateHostModule(ctx context.Context, mc wazero.ModuleConfig, meter *GasMeter, env *HostEnv) error {
    _, err := vm.runtime.NewHostModuleBuilder("dsn").
        NewFunctionBuilder().
        WithFunc(hostReadStorage).
        Export("read_storage").
        NewFunctionBuilder().
        WithFunc(hostWriteStorage).
        Export("write_storage").
        NewFunctionBuilder().
        WithFunc(hostEmitEvent).
        Export("emit_event").
        NewFunctionBuilder().
        WithFunc(hostReadCaller).
        Export("read_caller").
        NewFunctionBuilder().
        WithFunc(hostReadBlockHeight).
        Export("read_block_height").
        NewFunctionBuilder().
        WithFunc(hostReadBlockTimestamp).
        Export("read_block_timestamp").
        NewFunctionBuilder().
        WithFunc(hostTransferToken).
        Export("transfer_token").
        Instantiate(ctx)
    return err
}
```

### 7.5 Rejection of Unknown Imports

During module compilation, the VM inspects the WASM module's import section. If any import is not in the `"dsn"` module or calls an unknown function within it, compilation fails:

```go
func validateImports(compiled wazero.CompiledModule) error {
    for _, imp := range compiled.ImportedFunctions() {
        if imp.Module != "dsn" {
            return fmt.Errorf("%w: unknown module %q", types.ErrVMExecution, imp.Module)
        }
        // dsn module functions validated at host module instantiation
    }
    return nil
}
```

---

## 8. Events — `vm/events.go`

```go
type EventLog struct {
    events []types.Event
    mu     sync.Mutex
}

func NewEventLog() *EventLog
func (el *EventLog) Append(contractID types.Hash, topic string, data []byte)
func (el *EventLog) Drain() []types.Event

// SerializeEvents deterministically encodes events for inclusion in block data.
// Events are serialized in execution order.
func SerializeEvents(events []types.Event) ([]byte, error)  // uses types.BinaryMarshaler pattern
```

Events are:
- Deterministically serialized (binary BigEndian per existing convention)
- Included in the block as an optional field (NOT in the state root → cannot affect consensus)
- Replayable from genesis for audit/indexing

---

## 9. Contract Deployment — `vm/contract.go`

```go
// DeriveContractID computes a deterministic 32-byte contract identifier.
// contractID = SHA-256(deployer || nonce || wasmCodeHash)
func DeriveContractID(deployer types.Address, nonce uint64, wasmCodeHash types.Hash) types.Hash {
    buf := new(bytes.Buffer)
    buf.Write(deployer.Bytes())
    binary.Write(buf, binary.BigEndian, nonce)
    buf.Write(wasmCodeHash[:])
    return sha256.Sum256(buf.Bytes())  // returns [32]byte = types.Hash
}

// DeployContract handles the full deployment lifecycle.
func (vm *VM) DeployContract(tx *types.DeployContractTx, st ContractStore, blockHeight uint64) (*ExecutionResult, error) {
    // 1. Validate code size ≤ 1 MiB
    if len(tx.WASMCode) > 1024*1024 {
        return nil, types.ErrContractCodeTooLarge
    }

    // 2. Compute code hash
    codeHash := sha256.Sum256(tx.WASMCode)

    // 3. Derive contract ID
    contractID := DeriveContractID(tx.Sender, tx.Nonce, types.Hash(codeHash))

    // 4. Check if contract already exists
    if _, err := st.GetContractMeta(contractID); err == nil {
        return nil, fmt.Errorf("contract %x already exists", contractID)
    }

    // 5. Compile WASM module (rejects invalid/broken WASM)
    compiled, err := vm.runtime.CompileModule(vm.ctx, tx.WASMCode)
    if err != nil {
        return nil, fmt.Errorf("%w: compile: %v", types.ErrVMExecution, err)
    }
    defer compiled.Close(vm.ctx)

    // 6. Validate no unknown imports
    if err := validateImports(compiled); err != nil {
        return nil, err
    }

    // 7. Store code
    if err := st.SetCode(codeHash, tx.WASMCode); err != nil {
        return nil, err
    }

    // 8. Store metadata
    meta := &types.ContractMetadata{
        ContractID:   contractID,
        Deployer:     tx.Sender,
        CodeHash:     types.Hash(codeHash),
        DeployHeight: blockHeight,
        DeployNonce:  tx.Nonce,
    }
    if err := st.SetContractMeta(contractID, meta); err != nil {
        return nil, err
    }

    // 9. Call constructor (if exported)
    result, err := vm.callFunction(compiled, contractID, tx.Sender, tx.ConstructorArgs, "constructor", st, tx.GasLimit, blockHeight)
    if err != nil {
        return nil, err
    }

    return result, nil
}
```

---

## 10. State Integration — Extending `state.StateDB`

### 10.1 Extended StateDB Interface

```go
// state.StateDB gets new methods:
type StateDB interface {
    GetAccount(addr types.Address) (*Account, error)
    SetAccount(addr types.Address, account *Account) error
    DeleteAccount(addr types.Address) error
    Commit() (types.Hash, error)
    GetStateRoot() types.Hash

    // NEW — WASM contract state
    GetCode(codeHash types.Hash) ([]byte, error)
    SetCode(codeHash types.Hash, code []byte) error
    GetContractMeta(contractID types.Hash) (*types.ContractMetadata, error)
    SetContractMeta(contractID types.Hash, meta *types.ContractMetadata) error
    GetContractStorage(contractID types.Hash, key []byte) ([]byte, error)
    SetContractStorage(contractID types.Hash, key []byte, value []byte) error
}
```

### 10.2 InMemoryState Implementation

Add to `InMemoryState`:

```go
func (s *InMemoryState) GetCode(codeHash types.Hash) ([]byte, error) {
    val, ok := s.GetBytes("code:" + hex.EncodeToString(codeHash[:]))
    if !ok {
        return nil, types.ErrContractNotFound
    }
    return val, nil
}

func (s *InMemoryState) SetCode(codeHash types.Hash, code []byte) error {
    return s.SetBytes("code:"+hex.EncodeToString(codeHash[:]), code)
}

func (s *InMemoryState) GetContractMeta(contractID types.Hash) (*types.ContractMetadata, error) {
    val, ok := s.GetBytes("meta:" + hex.EncodeToString(contractID[:]))
    if !ok {
        return nil, types.ErrContractNotFound
    }
    var meta types.ContractMetadata
    // deserialize using gob or binary decoder
    return &meta, nil
}

func (s *InMemoryState) SetContractMeta(contractID types.Hash, meta *types.ContractMetadata) error {
    // serialize meta to bytes
    return s.SetBytes("meta:"+hex.EncodeToString(contractID[:]), encoded)
}

func (s *InMemoryState) GetContractStorage(contractID types.Hash, key []byte) ([]byte, error) {
    keyHash := sha256.Sum256(key)
    val, ok := s.GetBytes("storage:" + hex.EncodeToString(contractID[:]) + ":" + hex.EncodeToString(keyHash[:]))
    if !ok {
        return nil, fmt.Errorf("storage key not found")
    }
    return val, nil
}

func (s *InMemoryState) SetContractStorage(contractID types.Hash, key []byte, value []byte) error {
    if len(value) > 64*1024 {
        return fmt.Errorf("storage value exceeds 64 KiB limit")
    }
    keyHash := sha256.Sum256(key)
    return s.SetBytes("storage:"+hex.EncodeToString(contractID[:])+":"+hex.EncodeToString(keyHash[:]), value)
}
```

### 10.3 PersistentState Integration

The same key namespacing applies to the persistent (bbolt) implementation in `state/persistent.go`.

---

## 11. Consensus Integration

### 11.1 Transaction Discriminator

The existing `Transaction.Payload` is opaque bytes. To distinguish WASM contract txs from regular transactions, add a version-based discriminator:

```go
const (
    TxVersionTransfer  = 1  // existing: regular transfer
    TxVersionDeployWASM = 2 // new: DeployContractTx encoded as payload
    TxVersionCallWASM   = 3 // new: CallContractTx encoded as payload
)
```

The `DeployContractTx` and `CallContractTx` each implement a `ToPayload() []byte` method that encodes them into the `Transaction.Payload` field. The block builder checks `tx.Version` to decide how to execute.

### 11.2 Block Builder — `consensus/block_builder.go`

Modify `BuildBlock` to detect contract transactions:

```go
for _, tx := range txList {
    var receipt types.Hash

    switch tx.Version {
    case 1:
        receipt, err = state.ApplyTransaction(s, &tx, hasher, height)
    case 2, 3:
        var result *vm.ExecutionResult
        if tx.Version == 2 {
            deployTx := decodeDeployPayload(tx.Payload)
            result, err = vmInst.Execute(deployTx, s)
        } else {
            callTx := decodeCallPayload(tx.Payload)
            result, err = vmInst.Execute(callTx, s)
        }
        if err != nil {
            return nil, err
        }
        receipt = tx.IntentID
        // Collect events
        blockEvents = append(blockEvents, result.Events...)
    }

    if err != nil {
        return nil, err
    }
    totalFees += tx.MaxFee
    receipts = append(receipts, receipt)
}
```

### 11.3 Block Validator — `consensus/block_validator.go`

Same branching logic in `ValidateBlock`: re-execute each tx against a fresh state using the identical VM:

```go
for i := range block.Transactions {
    tx := &block.Transactions[i]
    switch tx.Version {
    case 1:
        _, err := state.ApplyTransaction(s, tx, hasher, block.Header.Height)
    case 2, 3:
        _, err = vmInst.Execute(decodedTx, s)
    }
    if err != nil {
        return fmt.Errorf("tx %x: %w", tx.IntentID, err)
    }
    totalFees += tx.MaxFee
}
```

### 11.4 Mempool Validation

Extend mempool `Submit` to validate contract txs:

```go
// In mempool.Submit, after basic validation:
switch tx.Version {
case 1:
    // existing checks
case 2: // deploy
    if err := validateDeployTx(tx, state); err != nil {
        return err
    }
case 3: // call
    if err := validateCallTx(tx, state); err != nil {
        return err
    }
}

func validateDeployTx(tx *types.Transaction, state BalanceChecker) error {
    deployTx, err := decodeDeployPayload(tx.Payload)
    if err != nil {
        return err
    }
    if len(deployTx.WASMCode) > 1024*1024 {
        return types.ErrContractCodeTooLarge
    }
    return nil
}

func validateCallTx(tx *types.Transaction, state BalanceChecker) error {
    callTx, err := decodeCallPayload(tx.Payload)
    if err != nil {
        return err
    }
    // Gas limit must be reasonable (≥ cost of one host call)
    if tx.GasLimit < 20 {
        return types.ErrGasLimitExceeded
    }
    return nil
}
```

---

## 12. Replay & Events

### 12.1 Deterministic Replay Guarantees

- **Same inputs → same outputs**: Given the same sequence of transactions at the same block height, block builder and block validator produce identical state roots.
- **Host functions are pure**: No IO, no randomness, no clock access — all inputs come from the block context (caller, height, timestamp).
- **Gas metering is deterministic**: Same WASM code + same inputs → same instruction count → same gas consumption.
- **Events don't affect state root**: Events are emitted during execution but stored only in block metadata. The state root depends only on SMT key-value pairs.

### 12.2 Event Storage in Block

Add an optional `ContractEvents` field to `types.Block`:

```go
// In types/block.go
type Block struct {
    Header         BlockHeader
    Transactions   []Transaction
    FeeSummary     FeeSummary
    Signature      []byte
    CommitProof    *CommitProof
    Evidence       []Evidence
    ContractEvents []Event  // NEW: deterministic event log
}

// Events are deterministically serialized for the block hash
// if they become part of consensus in a future upgrade.
func (b *Block) HeaderHash(hasher Hasher) (Hash, error) {
    // Events NOT included in current header hash — no consensus impact
}
```

---

## 13. Security Model

### 13.1 Contract Size Limit

```go
const MaxContractCodeSize = 1 * 1024 * 1024  // 1 MiB
```

Enforced at deployment (`vm/contract.go`) and mempool validation.

### 13.2 Storage Value Limit

```go
const MaxStorageValueSize = 64 * 1024  // 64 KiB
```

Enforced in `SetContractStorage`.

### 13.3 Memory Limit

```go
const WasmMemoryPages = 1  // 64 KiB (1 page = 64 KiB)
```

Enforced via `wazero.ModuleConfig.WithMemoryLimitPages(1)`.

### 13.4 Call Depth Limit

```go
const MaxCallDepth = 64
```

Enforced by a depth counter passed through `HostEnv`:

```go
type HostEnv struct {
    // ...
    CallDepth int
}

func (vm *VM) callFunction(compiled wazero.CompiledModule, ...) (*ExecutionResult, error) {
    if env.CallDepth >= MaxCallDepth {
        return nil, types.ErrCallDepthExceeded
    }
    env.CallDepth++
    defer func() { env.CallDepth-- }()
    // ... invoke WASM
}
```

### 13.5 Gas Limit Protection

Every `Execute` call creates a fresh `GasMeter(gasLimit)`. Each WASM instruction and host function call deducts from the meter. If exceeded, execution is aborted and state changes are discarded (the contract call reverts).

### 13.6 Import Rejection

Any WASM module importing functions outside the `"dsn"` module is rejected at compile time. This prevents contracts from calling system functions, reading files, or making network requests.

---

## 14. Backward Compatibility

1. **Existing transactions unaffected**: `Version == 1` transactions continue to go through `state.ApplyTransaction` unchanged.
2. **State root remains identical** for chains without contract txs: no new keys in SMT until a contract is deployed.
3. **Block format extended, not changed**: `ContractEvents` is additive — nil when no contract txs exist.
4. **Mempool evolution**: existing submit logic unchanged for `Version == 1`.
5. **Genesis backward compatible**: genesis block with no contracts produces identical state root.

---

## 15. File-by-File Implementation Plan

| Order | File | What |
|-------|------|------|
| 1 | `vm/gas.go` | GasMeter + gas constants |
| 2 | `vm/errors.go` | VM error vars + sentinel errors |
| 3 | `vm/events.go` | EventLog + SerializeEvents |
| 4 | `vm/storage.go` | ContractStoreBridge, key namespacing |
| 5 | `vm/runtime.go` | DeterministicConfig, NewRuntime |
| 6 | `vm/host.go` | HostEnv, all 7 host functions, module registration |
| 7 | `vm/contract.go` | DeriveContractID, deploy logic |
| 8 | `vm/vm.go` | VM struct, Execute, executeDeploy, executeCall, callFunction |
| 9 | `types/` additions | DeployContractTx, CallContractTx, ContractMetadata, Event, error vars, payload encode/decode |
| 10 | `state/` additions | InMemoryState.GetCode, .SetCode, .GetContractMeta, .SetContractMeta, .Get/SetContractStorage |
| 11 | `consensus/block_builder.go` | Discriminate tx by Version → call VM or state.ApplyTransaction |
| 12 | `consensus/block_validator.go` | Same discrimination for validation |
| 13 | `mempool/pool.go` | Validate deploy/call tx in Submit |
| 14 | Integration tests | Contract deploy → call → verify state root determinism |

---

## 16. Integration Test Patterns

Following the existing `TestDeterministicSmokeTest` pattern:

```go
func TestWASMContractDeployAndCall(t *testing.T) {
    hasher := types.SHA256Hasher{}
    st := state.NewInMemoryState(hasher)

    // 1. Fund deployer account
    deployerPub, deployerPriv, _ := ed25519.GenerateKey(nil)
    deployerAddr, _ := types.AddressFromBytes(deployerPub[:20])
    senderAccount := state.NewAccount(deployerAddr, [32]byte{})
    copy(senderAccount.PublicKey[:], deployerPub)
    st.SetAccount(deployerAddr, senderAccount)
    state.Mint(st, deployerAddr, types.NewAmount(1_000_000), hasher)
    st.Commit()

    // 2. Build DeployContractTx
    wasmCode := buildTestWASM() // compile tiny WASM with "dsn" imports
    deployTx := &types.DeployContractTx{
        Sender:    deployerAddr,
        Nonce:     1,
        WASMCode:  wasmCode,
        MaxFee:    1000,
        GasLimit:  100_000,
        Timestamp: uint64(time.Now().Unix()),
    }

    // 3. Execute deploy via VM
    ctx := context.Background()
    vmInst, _ := NewVM(ctx, hasher)
    result, err := vmInst.Execute(deployTx, st)
    require.NoError(t, err)
    require.False(t, result.Reverted)

    // 4. Verify contract exists in state
    contractID := DeriveContractID(deployerAddr, 1, sha256.Sum256(wasmCode))
    meta, err := st.GetContractMeta(contractID)
    require.NoError(t, err)
    require.Equal(t, deployerAddr, meta.Deployer)

    // 5. Build CallContractTx
    callTx := &types.CallContractTx{
        Sender:     deployerAddr,
        Nonce:      2,
        ContractID: contractID,
        Function:   "call",
        Args:       encodeArgs("hello"),
        MaxFee:     500,
        GasLimit:   50_000,
        Timestamp:  uint64(time.Now().Unix()),
    }

    // 6. Execute call
    result, err = vmInst.Execute(callTx, st)
    require.NoError(t, err)
    require.False(t, result.Reverted)
    require.Greater(t, result.GasUsed, uint64(0))

    // 7. Verify state root determinism
    root1, _ := st.Commit()
    // Replay on fresh state produces same root
    freshSt := state.NewInMemoryState(hasher)
    // ... recreate accounts + deploy + call
    root2, _ := freshSt.Commit()
    require.Equal(t, root1, root2, "state root must be deterministic")
}
```

---

## 17. Dependencies

Add to `go.mod`:

```
require (
    github.com/tetratelabs/wazero v1.8.0
)
```

wazero is a pure-Go WASM runtime with zero platform dependencies, deterministic execution, and no CGo — ideal for blockchain use.

---

## 18. Open Questions

1. **ABI encoding**: Define a standard ABI for WASM contract arguments (similar to Ethereum's ABI). Options: minimal binary encoding (type-length-value), or use `github.com/dsn/dsn/types` binary serialization patterns.
2. **Contract address vs ContractID**: Contracts that hold tokens need an Address (20 bytes). Proposal: `ContractAddress = Address(ContractID[:20])` — take first 20 bytes of the 32-byte ID and register as an account on first transfer.
3. **Revert semantics**: Should storage writes roll back on revert? Yes — the `ExecutionResult.Reverted = true` signals that all state mutations from the call are discarded. Implementation: snapshot state before call, restore on revert.
4. **WASM SDK**: Provide a TinyGo/Rust SDK with the `dsn` host import wrappers for contract developers.
